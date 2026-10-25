# Maintaining NetBirdy

NetBirdy is a fork of github.com/netbirdio/netbird with the dashboard vendored
as a subtree under dashboard/.

## Syncing upstream

scripts/sync-upstream.sh <tag> fetches the upstream tag, creates
sync/<tag>, merges it, re-applies the rebrand via tools/rebrand/rebrand.sh,
and runs the rebrand --check plus build and client tests. Merge conflicts are
printed for manual resolution. The dashboard subtree is updated separately:

  git subtree pull --prefix dashboard \
      https://github.com/netbirdio/netbird-dashboard.git <tag> --squash

The weekly upstream-sync workflow opens a sync/<tag> PR when upstream cuts a
new release.

## Rebrand rules

tools/rebrand/rebrand.sh holds the keep list: module path, imports, NB_* and
NETBIRD_* env vars, /etc/netbird, /var/lib/netbird, wt0, protocol identifiers,
io.netbird.* ids, upstream docs URLs, LICENSE and AUTHORS. Re-run it after any
upstream merge and keep --check green.

## Releases

Tagging vX.Y.Z or vX.Y.Z-nbyN (fork patch release) triggers
release-fork.yml: goreleaser builds assets and publishes the GitHub release,
and multi-arch images are pushed to ghcr.io/sudo-ivan/netbirdy-*. NetBirdy
versions mirror the upstream base and add -nbyN patch suffixes; docker tags
use the same string.

## Local preview

podman/setup.sh generates podman/.env and podman/config.yaml.
make podman-build builds both images, make podman-up starts the stack:
dashboard on :8080, combined server on :8081.
