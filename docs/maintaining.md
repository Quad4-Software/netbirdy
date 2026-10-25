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

## Published image flag

`test-docker-compose` in test-infrastructure-files.yml pulls the published
`ghcr.io/sudo-ivan/netbirdy-*` images. The job is gated behind the repository
variable `NETBIRDY_PUBLISHED_IMAGES=true`; set it after the first image
release so the job runs again.

## Release procedure

Pushing a `v*` tag runs `.github/workflows/release-fork.yml` (fork only). The
workflow_dispatch trigger is a dry run: it builds and signs everything, publishes
nothing and uploads the result as the `release-assets` workflow artifact.

Jobs:

- goreleaser builds the archives, deb and rpm packages and the client, relay,
  signal, management, upload and server images, and creates the GitHub release.
- windows-installer builds `netbirdy_<version>_windows_amd64` as an NSIS installer
  named `netbirdy_installer_<version>_windows_amd64.exe`. The installer ships the
  daemon and CLI without the desktop UI. No MSI is built.
- macos-pkg builds `netbirdy_<version>_darwin_amd64.pkg` and `_arm64.pkg`.
- images builds and pushes the dashboard and updater images and writes
  `netbirdy_<version>_images.txt` (server, dashboard and updater by digest).
- sign writes `netbirdy_<version>_checksums.txt`, signs every asset and the
  checksums and image list with the artifact key and verifies each signature
  against `publickeys/artifact-signatures/keys/artifact-key-pub.pem`.
- publish uploads the signatures and extra assets to the release, signs the
  checksums, image list and images with cosign, attests build provenance and
  marks stable releases latest.

Image tags have no leading v (`ghcr.io/sudo-ivan/netbirdy-server:1.2.3`).
Prerelease tags (a hyphen in the version, for example `v1.0.0-rc.1`) stay
GitHub prereleases and never move the `latest` image tag.

Required secret: `NETBIRDY_ARTIFACT_KEY` (the artifact private key). The root
private key never leaves `~/.config/netbirdy-signing/` on the maintainer
machine.
