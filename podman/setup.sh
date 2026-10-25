#!/bin/sh
# Generates podman/.env and podman/config.yaml for the local preview stack.
# Idempotent: existing generated files are kept unless --force is passed.
set -eu

PODMAN_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(dirname "$PODMAN_DIR")
ENV_FILE="$PODMAN_DIR/.env"
CONFIG_FILE="$PODMAN_DIR/config.yaml"
TEMPLATE="$PODMAN_DIR/config.yaml.tmpl"

if [ "${1:-}" = "--force" ]; then
    rm -f "$ENV_FILE" "$CONFIG_FILE"
fi

rand_b64() {
    openssl rand -base64 32 | tr -d '=\n'
}

rand_hex16() {
    openssl rand -hex 16
}

rand_password() {
    openssl rand -base64 24 | tr -d '=\n' | tr -d '/+'
}

bcrypt_hash() {
    if command -v go >/dev/null 2>&1; then
        (cd "$REPO_ROOT" && go run ./podman/genkey "$1")
    else
        podman run --rm \
            -v "$REPO_ROOT:/src:ro,Z" -w /src \
            docker.io/library/golang:1.27.2-bookworm \
            go run ./podman/genkey "$1"
    fi
}

if [ -f "$ENV_FILE" ] && [ -f "$CONFIG_FILE" ]; then
    . "$ENV_FILE"
    echo "podman preview is already configured (use --force to regenerate)."
    echo ""
    echo "  Dashboard:  http://localhost:8080"
    echo "  Server:     http://localhost:8081"
    echo "  OIDC:       http://localhost:8081/oauth2/.well-known/openid-configuration"
    echo "  Owner:      $NB_OWNER_EMAIL / $NB_OWNER_PASSWORD"
    exit 0
fi

NB_OWNER_EMAIL="${NB_OWNER_EMAIL:-admin@netbirdy.local}"
NB_OWNER_PASSWORD="${NB_OWNER_PASSWORD:-$(rand_password)}"
AUTH_SECRET=$(rand_b64)
SESSION_COOKIE_ENCRYPTION_KEY=$(rand_hex16)
STORE_ENCRYPTION_KEY=$(openssl rand -base64 32)
OWNER_PASSWORD_HASH=$(bcrypt_hash "$NB_OWNER_PASSWORD")

sed \
    -e "s|@AUTH_SECRET@|$AUTH_SECRET|g" \
    -e "s|@SESSION_COOKIE_ENCRYPTION_KEY@|$SESSION_COOKIE_ENCRYPTION_KEY|g" \
    -e "s|@STORE_ENCRYPTION_KEY@|$STORE_ENCRYPTION_KEY|g" \
    -e "s|@OWNER_EMAIL@|$NB_OWNER_EMAIL|g" \
    -e "s|@OWNER_PASSWORD_HASH@|$OWNER_PASSWORD_HASH|g" \
    "$TEMPLATE" > "$CONFIG_FILE"
chmod 0600 "$CONFIG_FILE"

cat > "$ENV_FILE" <<EOF
NB_OWNER_EMAIL=$NB_OWNER_EMAIL
NB_OWNER_PASSWORD=$NB_OWNER_PASSWORD
EOF
chmod 0600 "$ENV_FILE"

echo "Generated podman/config.yaml and podman/.env"
echo ""
echo "  Dashboard:  http://localhost:8080"
echo "  Server:     http://localhost:8081"
echo "  OIDC:       http://localhost:8081/oauth2/.well-known/openid-configuration"
echo "  Owner:      $NB_OWNER_EMAIL / $NB_OWNER_PASSWORD"
# Generate the updater bearer token once, never printed.
if ! grep -q '^NETBIRDY_UPDATER_TOKEN=' .env 2>/dev/null; then
  tok=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32)
  printf 'NETBIRDY_UPDATER_TOKEN=%s\n' "$tok" >> .env
  echo "generated NETBIRDY_UPDATER_TOKEN in .env"
fi
echo "to enable the updater profile: podman compose -f podman/compose.yaml --profile updater up -d"
