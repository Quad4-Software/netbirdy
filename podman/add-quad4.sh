#!/bin/sh
# Create a Quad4 ID identity provider through the management REST API.
# Usage: ./add-quad4.sh [quad4-url] <client-id> <client-secret> [api-key]
# The URL defaults to the public instance https://auth.quad4.io; pass it
# explicitly for a self-hosted Quad4 ID.
# Needs podman/.env NB_OWNER_TOKEN or a PAT passed as NB_TOKEN.
set -eu
case "${1:-}" in
  http://*|https://*) URL="$1"; shift ;;
  *) URL="https://auth.quad4.io" ;;
esac
CID="${1:?client id required}"
CSEC="${2:?client secret required}"
APIKEY="${3:-}"
MGMT="${NB_MGMT_URL:-http://localhost:8081}"
TOKEN="${NB_TOKEN:-$(grep -E '^NB_OWNER_TOKEN=' "$(dirname "$0")/.env" 2>/dev/null | cut -d= -f2-)}"
[ -z "$TOKEN" ] && { echo "no NB_TOKEN / NB_OWNER_TOKEN found" >&2; exit 1; }
case "$URL" in https://*|http://localhost*|http://127.0.0.1*) ;; *) echo "url must be https or localhost" >&2; exit 1;; esac
URL="${URL%/}"
curl -sf -X POST "$MGMT/api/identity-providers" \
  -H "Authorization: Token $TOKEN" -H "Content-Type: application/json" \
  -d "{\"type\":\"quad4\",\"name\":\"Quad4 ID\",\"issuer\":\"$URL\",\"client_id\":\"$CID\",\"client_secret\":\"$CSEC\"}" \
  | head -c 400; echo
[ -n "$APIKEY" ] && echo "note: quad4 id api key for user/group sync is configured separately in the provider edit screen" || true
