#!/bin/sh
# Writes the signed image list consumed by the updater.
# Usage: images-list.sh OUT PREFIX SERVER_DIGEST DASHBOARD_DIGEST UPDATER_DIGEST
# PREFIX is the registry path before the service name, for example
# ghcr.io/quad4-software/netbirdy-. Digests are sha256:<64 hex>.
set -eu

out=$1
prefix=$2
shift 2
[ "$#" -eq 3 ] || { echo "expected three digests" >&2; exit 2; }

: > "$out"
for pair in "server:$1" "dashboard:$2" "updater:$3"; do
    name=${pair%%:*}
    digest=${pair#*:}
    case "$digest" in
        sha256:*) hex=${digest#sha256:} ;;
        *) echo "digest for $name must start with sha256:" >&2; exit 1 ;;
    esac
    if ! printf '%s' "$hex" | grep -Eq '^[0-9a-f]{64}$'; then
        echo "digest for $name is not 64 lowercase hex characters" >&2
        exit 1
    fi
    printf '%s%s@%s\n' "$prefix" "$name" "$digest" >> "$out"
done
