#!/bin/sh
# Tests for the release helper scripts. Needs the signer binary in SIGNER.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
: "${SIGNER:?SIGNER must point at the signer binary}"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

sh "$here/version_test.sh" > /dev/null || fail "version tests"

"$SIGNER" create-root-key --priv-key-file "$work/root.key" --pub-key-file "$work/root.pub" --expiration 24h > /dev/null
"$SIGNER" create-artifact-key --root-private-key-file "$work/root.key" --artifact-priv-key-file "$work/art.key" \
    --artifact-pub-key-file "$work/art.pub" --expiration 24h > /dev/null

assets="$work/assets"
mkdir -p "$assets"
printf 'installer' > "$assets/netbirdy_installer_1.2.3_windows_amd64.exe"
printf 'pkg' > "$assets/netbirdy_1.2.3_darwin_arm64.pkg"
printf 'ghcr.io/x/netbirdy-server@sha256:%064d\n' 0 > "$assets/netbirdy_1.2.3_images.txt"
printf '%s  %s\n' "$(printf 'other' | sha256sum | cut -d' ' -f1)" "install.sh" > "$work/goreleaser_checksums.txt"

sh "$here/sign.sh" "$assets" 1.2.3 "$work/art.key" "$work/art.pub" "$work/goreleaser_checksums.txt" > /dev/null || fail "sign.sh"

sums="$assets/netbirdy_1.2.3_checksums.txt"
[ -f "$sums" ] || fail "checksums file missing"
grep -q '  install.sh$' "$sums" || fail "merged checksum line was dropped"
grep -q '  netbirdy_1.2.3_darwin_arm64.pkg$' "$sums" || fail "pkg checksum missing"
grep -q 'checksums.txt$' "$sums" && fail "checksums file lists itself"
( cd "$assets" && grep -v ' install.sh$' "$sums" | sha256sum -c - > /dev/null ) || fail "checksums do not verify"
for f in netbirdy_installer_1.2.3_windows_amd64.exe netbirdy_1.2.3_darwin_arm64.pkg netbirdy_1.2.3_images.txt netbirdy_1.2.3_checksums.txt; do
    [ -f "$assets/$f.sig" ] || fail "missing signature for $f"
    "$SIGNER" verify-artifact --artifact-public-key-file "$work/art.pub" --artifact-file "$assets/$f" --signature-file "$assets/$f.sig" > /dev/null || fail "verify $f"
done
[ -f "$assets/install.sh.sig" ] && fail "signed a file that is not in the directory"

printf 'tampered' > "$assets/netbirdy_1.2.3_darwin_arm64.pkg"
if "$SIGNER" verify-artifact --artifact-public-key-file "$work/art.pub" --artifact-file "$assets/netbirdy_1.2.3_darwin_arm64.pkg" \
    --signature-file "$assets/netbirdy_1.2.3_darwin_arm64.pkg.sig" > /dev/null 2>&1; then
    fail "tampered file verified"
fi

d=$(printf '%064d' 7)
sh "$here/images-list.sh" "$work/images.txt" ghcr.io/quad4-software/netbirdy- "sha256:$d" "sha256:$d" "sha256:$d"
[ "$(wc -l < "$work/images.txt")" -eq 3 ] || fail "images list line count"
grep -q "^ghcr.io/quad4-software/netbirdy-updater@sha256:$d\$" "$work/images.txt" || fail "images list format"
if sh "$here/images-list.sh" "$work/bad.txt" p- "sha256:abc" "sha256:$d" "sha256:$d" 2> /dev/null; then fail "short digest accepted"; fi
if sh "$here/images-list.sh" "$work/bad.txt" p- "sha256:$(printf 'A%.0s' $(seq 64))" "sha256:$d" "sha256:$d" 2> /dev/null; then fail "uppercase digest accepted"; fi
if sh "$here/images-list.sh" "$work/bad.txt" p- "$d" "sha256:$d" "sha256:$d" 2> /dev/null; then fail "missing sha256 prefix accepted"; fi

echo "release scripts ok"
