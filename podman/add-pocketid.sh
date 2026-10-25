#!/bin/sh
# Create a Pocket ID identity provider through the management REST API.
# Usage: ./add-pocketid.sh <pocketid-url> <client-id> <client-secret> [api-key]
# Needs podman/.env NB_OWNER_TOKEN or a PAT passed as NB_TOKEN.
set -eu
URL="${1:?pocketid base url required}"
CID="${2:?client id required}"
CSEC="${3:?client secret required}"
APIKEY="${4:-}"
MGMT="${NB_MGMT_URL:-http://localhost:8081}"
TOKEN="${NB_TOKEN:-$(grep -E '^NB_OWNER_TOKEN=' "$(dirname "$0")/.env" 2>/dev/null | cut -d= -f2-)}"
[ -z "$TOKEN" ] && { echo "no NB_TOKEN / NB_OWNER_TOKEN found" >&2; exit 1; }
case "$URL" in https://*|http://localhost*|http://127.0.0.1*) ;; *) echo "url must be https or localhost" >&2; exit 1;; esac
URL="${URL%/}"
curl -sf -X POST "$MGMT/api/identity-providers" \
  -H "Authorization: Token $TOKEN" -H "Content-Type: application/json" \
  -d "{\"type\":\"pocketid\",\"name\":\"PocketID\",\"issuer\":\"$URL\",\"client_id\":\"$CID\",\"client_secret\":\"$CSEC\"}" \
  | head -c 400; echo
[ -n "$APIKEY" ] && echo "note: pocket id api key for user/group sync is configured separately in the provider edit screen" || true
