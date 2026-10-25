#!/bin/sh
# Fails when a foreign lockfile exists or the pnpm lockfile is stale.
set -eu
cd "$(dirname "$0")/.."
bad=$(find . -name node_modules -prune -o \( -name package-lock.json -o -name yarn.lock \) -print)
if [ -n "$bad" ]; then
  echo "foreign lockfiles found:" >&2
  echo "$bad" >&2
  exit 1
fi
tmp=$(mktemp)
cp pnpm-lock.yaml "$tmp"
trap 'cp "$tmp" pnpm-lock.yaml; rm -f "$tmp"' EXIT
if ! pnpm install --frozen-lockfile --lockfile-only >/dev/null 2>&1; then
  echo "pnpm-lock.yaml is stale; run pnpm install and commit the result" >&2
  exit 1
fi
