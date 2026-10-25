#!/bin/sh
# Writes the checksums file for a release asset directory and signs every asset.
# Usage: sign.sh DIR VERSION KEY_FILE PUB_FILE [EXISTING_CHECKSUMS]
# SIGNER must point at the signer binary built from client/cmd/signer.
# VERSION has no leading v. EXISTING_CHECKSUMS lines are kept for files that are
# not in DIR, so checksums of assets published by another step are preserved.
set -eu

dir=$1
ver=$2
key=$3
pub=$4
merge=${5:-}
: "${SIGNER:?SIGNER must point at the signer binary}"

dir=$(cd "$dir" && pwd)
case "$key" in /*) ;; *) key="$PWD/$key" ;; esac
case "$pub" in /*) ;; *) pub="$PWD/$pub" ;; esac
[ -n "$merge" ] && case "$merge" in /*) ;; *) merge="$PWD/$merge" ;; esac

cd "$dir"
sums="netbirdy_${ver}_checksums.txt"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

list_assets() {
    find . -maxdepth 1 -type f ! -name '*.sig' ! -name '*.bundle' ! -name '*_checksums.txt' -printf '%f\n' | sort
}

: > "$tmp"
if [ -n "$merge" ] && [ -f "$merge" ]; then
    cat "$merge" >> "$tmp"
fi
list_assets | while IFS= read -r f; do
    sha256sum "$f" >> "$tmp"
done
sort -u -k2,2 "$tmp" > "$sums"

count=0
for f in $(list_assets) "$sums"; do
    "$SIGNER" sign-artifact --artifact-key-file "$key" --artifact-file "$f" > /dev/null
    "$SIGNER" verify-artifact --artifact-public-key-file "$pub" --artifact-file "$f" --signature-file "$f.sig" > /dev/null
    count=$((count + 1))
done
printf 'signed and verified %s files, checksums in %s\n' "$count" "$sums"
