#!/bin/sh
# offline-e2e.sh - throwaway two-client offline-mode test rig.
# Subcommands: up | status | scenario <name> | down
# Scenarios: running restart-one restart-both ip-change server-back revoke
# Everything created is named nbyoff-*; down removes only those resources.

set -eu

NET=nbyoff-net
SUBNET=10.89.77.0/24
SRV=nbyoff-srv
SRV_IP=10.89.77.10
A=nbyoff-a
A_IP=10.89.77.21
B=nbyoff-b
B_IP=10.89.77.22
DIR=/tmp/nbyoff
OUT=/tmp/netbirdy-audit/offline
SRV_IMG=${NB_E2E_SERVER_IMG:-localhost/netbirdy-server:latest}
CLI_IMG=${NB_E2E_CLIENT_IMG:-localhost/netbirdy-client:dev}
PORT=18081
OVERLAY_A=""
OVERLAY_B=""

mkdir -p "$DIR" "$OUT"

say() { printf '%s\n' "$*"; }

require() { command -v podman >/dev/null || { say "podman required"; exit 1; }; }

wait_oidc() {
  i=0
  while [ $i -lt 40 ]; do
    if curl -sf "http://127.0.0.1:$PORT/oauth2/.well-known/openid-configuration" -o /dev/null; then return 0; fi
    i=$((i + 1)); sleep 2
  done
  say "OIDC discovery did not come up"; return 1
}

render_config() {
  [ -f "$DIR/config.yaml" ] && return 0
  sed -e "s|:33073|:$PORT|g" \
      -e "s|exposedAddress:.*|exposedAddress: \"http://$SRV:$PORT\"|" \
      -e "s|issuer:.*|issuer: \"http://$SRV:$PORT/oauth2\"|" \
      -e "s|admin@netbirdy.local|admin@nbyoff.local|g" \
      podman/config.yaml.tmpl > "$DIR/config.yaml" 2>/dev/null || {
    # fall back to a minimal combined config matching the O4a shape
    cat > "$DIR/config.yaml" <<EOF
server:
  listenAddress: ":$PORT"
  exposedAddress: "http://$SRV:$PORT"
  stunPorts: [3478]
  metricsPort: 9090
  healthcheckAddress: ":9000"
  logLevel: "info"
  logFile: "console"
  authSecret: "eSShUYOwiepn0ozXa9Tf4APanFUKhS3"
  dataDir: "/var/lib/netbird"
  disableAnonymousMetrics: true
  disableGeoliteUpdate: true
  auth:
    issuer: "http://$SRV:$PORT/oauth2"
    signKeyRefreshEnabled: true
    sessionCookieEncryptionKey: "3713079cc55874babda45f2e8e0b95b9"
    dashboardRedirectURIs:
      - "http://localhost:8080/nb-auth"
      - "http://localhost:8080/nb-silent-auth"
    dashboardPostLogoutRedirectURIs:
      - "http://localhost:8080/"
    cliRedirectURIs:
      - "http://localhost:53000/"
    owner:
      email: "admin@nbyoff.local"
      password: "k3SGmuzxTVBlgEJU4oB44lCOVX25VafU"
  store:
    engine: "sqlite"
    encryptionKey: "Nx4so6ITeMWMT+KwSlJFBP5NWwQ34Sl26vcr/dX2qh8="
EOF
  }
}

get_token() {
  # Dex auth-code flow through the local connector: /auth -> /auth/local ->
  # POST /auth/local/login -> redirect straight to the code. Password and
  # refresh grants are off.
  [ -s "$DIR/owner.pass" ] || return 1
  PW=$(cat "$DIR/owner.pass")
  JAR="$DIR/cookies.txt"; : > "$JAR"
  REDIR="http%3A%2F%2Flocalhost%3A53000%2F"
  L1=$(curl -s -c "$JAR" -o /dev/null -w "%{redirect_url}" \
      "http://127.0.0.1:$PORT/oauth2/auth?client_id=netbird-cli&redirect_uri=$REDIR&response_type=code&scope=openid&state=x" \
      | sed "s|http://$SRV:$PORT|http://127.0.0.1:$PORT|")
  [ -n "$L1" ] || return 1
  L2=$(curl -s -b "$JAR" -c "$JAR" -o /dev/null -w "%{redirect_url}" "$L1" \
      | sed "s|http://$SRV:$PORT|http://127.0.0.1:$PORT|")
  [ -n "$L2" ] || return 1
  L3=$(curl -s -b "$JAR" -c "$JAR" -o /dev/null -w "%{redirect_url}" \
      -d "login=admin@nbyoff.local" -d "password=$PW" "$L2")
  CODE=$(printf %s "$L3" | sed -n 's/.*[?&]code=\([^&]*\).*/\1/p')
  [ -n "$CODE" ] || return 1
  curl -s -X POST "http://127.0.0.1:$PORT/oauth2/token" \
      -d "grant_type=authorization_code&code=$CODE&redirect_uri=http://localhost:53000/&client_id=netbird-cli" \
      > "$DIR/token.json"
  grep -o '"access_token":"[^"]*"' "$DIR/token.json" | cut -d'"' -f4 > "$DIR/api.token"
  [ -s "$DIR/api.token" ]
}

api() { curl -sf -H "Authorization: Bearer $(cat "$DIR/api.token")" "$@"; }

overlay_ip() {
  podman exec "$1" netbirdy status --json 2>/dev/null | grep -o '"netbirdIp":"[^"]*"' | cut -d'"' -f4 | cut -d/ -f1 | head -1
}

wait_connected() {
  i=0
  while [ $i -lt 40 ]; do
    if podman exec "$1" netbirdy status 2>/dev/null | grep -q "Management: Connected"; then return 0; fi
    i=$((i + 1)); sleep 2
  done
  say "$1 did not reach Connected"; return 1
}

ping_ab() {
  OVERLAY_B=${OVERLAY_B:-$(overlay_ip "$B")}
  podman exec "$A" ping -c 3 -W 3 "$OVERLAY_B" > "$OUT/$1-ping.txt" 2>&1 && return 0
  return 1
}

result() { say "$2: $1"; }

client_run() {
  name=$1 ip=$2 host=$3
  podman run -d --name "$name" --network "$NET" --ip "$ip" \
    --cap-add NET_ADMIN --cap-add NET_RAW --device /dev/net/tun \
    --hostname "$host" \
    -e NB_SETUP_KEY="$(cat "$DIR/setup.key")" \
    -e NB_MANAGEMENT_URL="http://$SRV:$PORT" \
    -e NB_HOSTNAME="$host" \
    -e NB_OFFLINE_START_DELAY=3 \
    -e NB_ENTRYPOINT_SKIP_UP=1 \
    -e NB_LOG_LEVEL=debug \
    -v "$name-state:/var/lib/netbird" \
    "$CLI_IMG" >/dev/null
}

cmd_up() {
  require
  podman network exists "$NET" 2>/dev/null || podman network create --subnet "$SUBNET" "$NET" >/dev/null
  if ! podman container exists "$SRV"; then
    render_config
    podman run -d --name "$SRV" --network "$NET" --ip "$SRV_IP" \
      -p "127.0.0.1:$PORT:$PORT" \
      -v "$DIR/config.yaml:/etc/netbird/config.yaml:ro" \
      -v nbyoff-data:/var/lib/netbird \
      "$SRV_IMG" >/dev/null
  fi
  wait_oidc
  # The config seeds the owner password field on every boot and leaves a
  # malformed hash, so re-set it on every `up` with the flag form of
  # admin change-password (positional args write a malformed hash too).
  [ -s "$DIR/owner.pass" ] || echo 'E2e-pass-123!' > "$DIR/owner.pass"
  podman exec "$SRV" /go/bin/netbirdy-server -c /etc/netbird/config.yaml \
    admin user change-password --email admin@nbyoff.local \
    --password "$(cat "$DIR/owner.pass")" >/dev/null 2>&1 || true
  if ! get_token; then say "token flow failed"; exit 1; fi
  api -X POST "http://127.0.0.1:$PORT/api/setup-keys" -H "Content-Type: application/json" \
    -d '{"type":"reusable","expires_in":604800,"name":"e2e"}' > "$DIR/key.json"
  grep -o '"key":"[^"]*"' "$DIR/key.json" | cut -d'"' -f4 > "$DIR/setup.key"
  [ -s "$DIR/setup.key" ]
  podman rm -f "$A" "$B" >/dev/null 2>&1 || true
  client_run "$A" "$A_IP" nbyoff-a
  client_run "$B" "$B_IP" nbyoff-b
  sleep 5
  # entrypoint skips `netbirdy up` (NB_ENTRYPOINT_SKIP_UP=1) so a failed
  # up cannot call Down() and kill a later offline engine; bring them up here.
  podman exec "$A" netbirdy up --setup-key "$(cat "$DIR/setup.key")" >/dev/null 2>&1 || true
  podman exec "$B" netbirdy up --setup-key "$(cat "$DIR/setup.key")" >/dev/null 2>&1 || true
  wait_connected "$A"
  wait_connected "$B"
  # Warm up the direct path both ways: the pair only forms on first
  # traffic and endpoint capture only fires on a live transition.
  OVERLAY_B=$(overlay_ip "$B"); OVERLAY_A=$(overlay_ip "$A")
  podman exec "$A" ping -c 4 -W 2 "$OVERLAY_B" >/dev/null 2>&1 || true
  podman exec "$B" ping -c 4 -W 2 "$OVERLAY_A" >/dev/null 2>&1 || true
  # Wait until the snapshot carries at least one direct endpoint (stage 2
  # needs them; a relayed or unestablished pair writes none).
  i=0
  while [ $i -lt 45 ]; do
    a_ok=$(podman exec "$A" sh -c 'cat /var/lib/netbird/networkmap.bin 2>/dev/null' | grep -c '"endpoints":{"' || true)
    b_ok=$(podman exec "$B" sh -c 'cat /var/lib/netbird/networkmap.bin 2>/dev/null' | grep -c '"endpoints":{"' || true)
    if [ "$a_ok" -gt 0 ] && [ "$b_ok" -gt 0 ]; then break; fi
    i=$((i + 1)); sleep 2
  done
  OVERLAY_A=$(overlay_ip "$A"); OVERLAY_B=$(overlay_ip "$B")
  say "up: A=$OVERLAY_A B=$OVERLAY_B"
}

cmd_status() {
  for c in "$A" "$B"; do
    podman container exists "$c" && podman exec "$c" netbirdy status | grep -E "Management:|Signal:|Peers count" && say "---"
  done
}

run_scenario() {
  name=$1
  OVERLAY_B=$(overlay_ip "$B" 2>/dev/null || true)
  case "$name" in
    running)
      podman stop "$SRV" >/dev/null; sleep 60
      podman exec "$A" netbirdy status > "$OUT/running-status.txt" 2>&1 || true
      if ping_ab running && grep -q "Management: Disconnected" "$OUT/running-status.txt"; then result PASS running
      else result "FAIL (peers dropped or status wrong, see $OUT/running-*)" running; fi
      podman start "$SRV" >/dev/null; sleep 30
      ;;
    restart-one)
      podman stop "$SRV" >/dev/null; podman restart "$A" >/dev/null
      i=0; while [ $i -lt 30 ]; do
        podman exec "$A" netbirdy status 2>/dev/null | grep -qi "Offline" && break
        i=$((i + 1)); sleep 3
      done
      podman logs "$A" > "$OUT/restart-one-logs.txt" 2>&1
      podman exec "$A" netbirdy status > "$OUT/restart-one-status.txt" 2>&1 || true
      if ping_ab restart-one; then result PASS restart-one
      else result "FAIL ($(grep -m1 'offline decision\|handshake\|endpoint' "$OUT/restart-one-logs.txt" || echo 'ping failed')" restart-one; fi
      podman start "$SRV" >/dev/null; sleep 30
      ;;
    restart-both)
      podman stop "$SRV" >/dev/null
      podman restart "$A" "$B" >/dev/null
      i=0; while [ $i -lt 30 ]; do
        podman exec "$A" netbirdy status 2>/dev/null | grep -qi "Offline" && break
        i=$((i + 1)); sleep 3
      done
      podman logs "$A" > "$OUT/restart-both-logs-a.txt" 2>&1
      podman logs "$B" > "$OUT/restart-both-logs-b.txt" 2>&1
      if ping_ab restart-both; then result PASS restart-both
      else result "FAIL ($(grep -m1 'offline decision\|handshake\|dns\|lookup' "$OUT/restart-both-logs-a.txt" || echo 'ping failed'))" restart-both; fi
      podman start "$SRV" >/dev/null; sleep 30
      ;;
    ip-change)
      podman stop "$SRV" >/dev/null
      podman network disconnect "$NET" "$A"; podman network disconnect "$NET" "$B"
      podman network connect --ip 10.89.77.31 "$NET" "$A" 2>/dev/null || podman network connect "$NET" "$A"
      podman network connect --ip 10.89.77.32 "$NET" "$B" 2>/dev/null || podman network connect "$NET" "$B"
      podman restart "$A" "$B" >/dev/null
      i=0; while [ $i -lt 30 ]; do
        podman exec "$A" netbirdy status 2>/dev/null | grep -qi "Offline" && break
        i=$((i + 1)); sleep 3
      done
      podman logs "$A" > "$OUT/ip-change-logs.txt" 2>&1
      if ping_ab ip-change; then result PASS ip-change
      else result "FAIL (stored endpoints stale after container IP change, see $OUT/ip-change-*)" ip-change; fi
      podman start "$SRV" >/dev/null; sleep 30
      ;;
    server-back)
      podman start "$SRV" >/dev/null 2>&1 || true
      # dial attempts are slow (context deadline per target); allow 3 min
      i=0
      while [ $i -lt 60 ]; do
        if podman exec "$A" netbirdy status 2>/dev/null | grep -q "Management: Connected" &&            podman exec "$B" netbirdy status 2>/dev/null | grep -q "Management: Connected"; then
          result PASS server-back; return 0
        fi
        i=$((i + 1)); sleep 3
      done
      result "FAIL (clients still offline after 180s)" server-back
      ;;
    revoke)
      podman stop "$SRV" >/dev/null; podman restart "$A" >/dev/null; sleep 12
      podman network disconnect "$NET" "$A"
      podman start "$SRV" >/dev/null; sleep 15
      # delete peer A specifically: match the peer whose ip is A's overlay
      OVERLAY_A=${OVERLAY_A:-$(overlay_ip "$A" 2>/dev/null || true)}
      PID=$(api "http://127.0.0.1:$PORT/api/peers" | tr '}' '\n' | grep "\"ip\":\"$OVERLAY_A" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
      if [ -z "$PID" ]; then
        PID=$(api "http://127.0.0.1:$PORT/api/peers" | tr '}' '\n' | grep -i "nbyoff-a" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
      fi
      if [ -n "$PID" ]; then api -X DELETE "http://127.0.0.1:$PORT/api/peers/$PID" >/dev/null; fi
      podman network connect --ip "$A_IP" "$NET" "$A" 2>/dev/null || podman network connect "$NET" "$A"
      sleep 20
      podman logs "$A" > "$OUT/revoke-logs.txt" 2>&1
      podman exec "$A" ls /var/lib/netbird/ > "$OUT/revoke-state.txt" 2>&1 || true
      if grep -q "PermissionDenied\|Unauthenticated" "$OUT/revoke-logs.txt" && ! grep -q "networkmap.bin" "$OUT/revoke-state.txt"; then
        result PASS revoke
      else result "FAIL (snapshot survived or no auth refusal, see $OUT/revoke-*)" revoke; fi
      ;;
    *) say "unknown scenario $name"; exit 1 ;;
  esac
}

cmd_down() {
  for c in "$A" "$B" "$SRV"; do podman rm -f "$c" >/dev/null 2>&1 || true; done
  for v in nbyoff-a-state nbyoff-b-state nbyoff-data; do podman volume rm "$v" >/dev/null 2>&1 || true; done
  podman network rm "$NET" >/dev/null 2>&1 || true
  say "down: nbyoff-* removed"
}

case "${1:-}" in
  up) cmd_up ;;
  status) cmd_status ;;
  scenario)
    name=${2:-}
    verdict=$(run_scenario "$name" 2>&1) || true
    if printf %s "$verdict" | grep -q "^$name: PASS"; then
      printf %s "$verdict" | grep "^$name:"
    else
      printf %s "$verdict" | tail -3
      say "$name: FAIL (scenario exited without PASS, logs in $OUT/$name-*)"
    fi
    ;;
  down) cmd_down ;;
  *) say "usage: $0 {up|status|scenario NAME|down}"; exit 1 ;;
esac
