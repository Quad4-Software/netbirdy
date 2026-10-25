#!/bin/sh
# Re-apply the NetBirdy rebrand after an upstream merge.
# Usage: tools/rebrand/rebrand.sh [--check]
#   default: rewrite user-visible NetBird strings to NetBirdy
#   --check: exit non-zero when new upstream brand strings slipped in
#
# KEEP list (never rewritten): the Go module path github.com/netbirdio/netbird,
# import paths, NB_* and NETBIRD_* environment variables, /etc/netbird,
# /var/lib/netbird, /var/run/netbird*, wt0, gRPC and proto identifiers, the
# OIDC client id netbird-dashboard, io.netbird.* bundle and policy ids,
# firewall and nftables chain names, upstream URLs under *.netbird.io that
# document upstream behaviour or identify persisted state, LICENSE, AUTHORS
# and NetBird GmbH attribution strings, generated *.pb.go files.
set -eu

cd "$(dirname "$0")/../.."
CHECK=0
[ "${1:-}" = "--check" ] && CHECK=1

EXCLUDE='--exclude-dir=.git --exclude-dir=node_modules --exclude-dir=vendor --exclude=*.pb.go --exclude-dir=frontend-ui --exclude-dir=testdata --exclude-dir=fixtures --exclude-dir=golden --exclude=*_golden.* --exclude=*.golden'

# Word-boundary display name rewrite in user-visible text. Identifiers such as
# isNetBirdCloud stay untouched because of the word boundaries.
if [ "$CHECK" -eq 0 ]; then
    grep -rln $EXCLUDE 'NetBird\|Netbird' . \
        | xargs -r sed -i 's/\bNetBird\b/NetBirdy/g; s/\bNetbird\b/NetBirdy/g'
    exit 0
fi

# --check: report any remaining bare NetBird display strings outside the keep
# zones below. Each pattern is a known-allowed context.
hits=$(grep -rn $EXCLUDE '\bNetBird\b\|\bNetbird\b' . \
    | grep -v 'netbirdio/netbird' \
    | grep -v 'github\.com/netbirdio' \
    | grep -v 'netbird\.io' \
    | grep -v 'NetBird GmbH' \
    | grep -v 'LICENSE' \
    | grep -v 'AUTHORS' \
    | grep -v 'NOTICE' \
    | grep -v '\.test\.' \
    | grep -v '_test\.go' \
    | grep -v '\.proto' \
    | grep -v 'tools/rebrand/' \
    | grep -v 'fork of NetBird' \
    | grep -v 'NetBird service' \
    | grep -v 'netbird service' \
    | grep -v 'docs/netbird' \
    | grep -v 'content/docs/upstream' \
    | grep -v 'docs-img' \
    | grep -v 'dashboard/out/' \
    | grep -v 'content/docs/project' \
    | grep -v 'dashboard/.next' \
    | grep -v 'import-upstream-docs' \
    | grep -v 'src/generated' \
    | grep -v 'testdata' \
    | grep -v 'io.netbird' \
    | grep -v 'migrating-from-netbird' \
    | grep -v 'sync-upstream' \
    | grep -v 'upstream NetBird' \
    | grep -v 'funding' \
    | grep -v 'netbird install' \
    | grep -v 'NetBird install' \
    | grep -v '^./README\.md:' \
    | grep -v 'official NetBird mobile apps' \
    || true)

if [ -n "$hits" ]; then
    printf '%s\n' "$hits"
    printf 'rebrand check failed: upstream brand strings found\n' >&2
    exit 1
fi
printf 'rebrand check ok\n'
