#!/bin/sh
# Writes the digest references for every published container image variant.
# Usage: image-provenance-list.sh OUT PREFIX IMAGE DIGEST [IMAGE DIGEST ...]
set -eu

out=$1
prefix=$2
base=${prefix%-}
shift 2
[ "$#" -gt 0 ] && [ $(( $# % 2 )) -eq 0 ] || {
    echo "expected image and digest pairs" >&2
    exit 2
}

tmp="$out.tmp"
trap 'rm -f "$tmp"' EXIT HUP INT TERM
: > "$tmp"
while [ "$#" -gt 0 ]; do
    image=$1
    digest=$2
    shift 2
    case "$image" in
        "$base"|"$prefix"dashboard|"$prefix"management|"$prefix"nblink|\
        "$prefix"relay|"$prefix"reverse-proxy|"$prefix"server|\
        "$prefix"signal|"$prefix"updater|"$prefix"upload) ;;
        *) echo "image $image is outside the published repository set" >&2; exit 1 ;;
    esac
    case "$digest" in
        sha256:*) hex=${digest#sha256:} ;;
        *) echo "digest for $image must start with sha256:" >&2; exit 1 ;;
    esac
    if ! printf '%s' "$hex" | grep -Eq '^[0-9a-f]{64}$'; then
        echo "digest for $image is not 64 lowercase hex characters" >&2
        exit 1
    fi
    if grep -Fq "$image@$digest" "$tmp"; then
        echo "duplicate image digest: $image@$digest" >&2
        exit 1
    fi
    printf '%s@%s\n' "$image" "$digest" >> "$tmp"
done

mv "$tmp" "$out"
trap - EXIT HUP INT TERM
