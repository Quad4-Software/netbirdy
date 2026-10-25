#!/bin/sh
# Sync an upstream NetBird release into this fork.
# Usage: scripts/sync-upstream.sh <upstream-tag>
set -eu
TAG="${1:?usage: sync-upstream.sh vX.Y.Z}"
BRANCH="sync/${TAG}"
git fetch https://github.com/netbirdio/netbird.git "refs/tags/${TAG}:refs/tags/${TAG}"
git checkout -b "$BRANCH"
if ! git merge --no-edit "$TAG"; then
    echo "merge conflicts in:" >&2
    git diff --name-only --diff-filter=U
    exit 2
fi
tools/rebrand/rebrand.sh
tools/rebrand/rebrand.sh --check
# dashboard lives in a subtree; bring it forward separately:
#   git subtree pull --prefix dashboard https://github.com/netbirdio/netbird-dashboard.git <tag> --squash
go build ./client/... ./combined/... ./management/... ./signal/... ./relay/... ./proxy/... ./upload-server/...
go test -tags devcert ./client/cmd/... ./combined/...
echo "sync branch $BRANCH ready; review, then open a PR"
