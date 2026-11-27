# Maintaining NetBirdy

NetBirdy is a fork of `github.com/netbirdio/netbird`. The dashboard is
vendored as a subtree under `dashboard/`.

## Syncing with NetBird

Run:

```sh
scripts/sync-upstream.sh <tag>
```

The script fetches the upstream tag, creates `sync/<tag>`, merges it, applies
the rebrand, and runs the rebrand check plus build and client tests. Resolve
printed conflicts by hand.

Update the dashboard subtree separately:

```sh
git subtree pull --prefix dashboard \
  https://github.com/netbirdio/netbird-dashboard.git <tag> --squash
```

The weekly `upstream-sync` workflow can prepare the same branch when NetBird
publishes a new tag.

## Rebrand rules

`tools/rebrand/rebrand.sh` contains the keep list: the module path, imports,
`NB_*` and `NETBIRD_*` variables, `/etc/netbird`, `/var/lib/netbird`, `wt0`,
protocol identifiers, `io.netbird.*` identifiers, documentation URLs, license
files, and author files. Run the script after every upstream merge and keep
`--check` green.

## Releases

A `vX.Y.Z` tag builds a normal release. A `vX.Y.Z-nbyN` tag builds a fork
patch release. Both trigger `.github/workflows/release-fork.yml`.

The workflow builds client archives and packages, server images, desktop
packages, the Windows installer, and macOS packages. It signs the assets,
publishes signatures and attestations, and pushes multi-architecture images
to `ghcr.io/quad4-software`.

A manual workflow dispatch is a dry run. It builds and signs the release but
publishes nothing. The output is uploaded as the `release-assets` workflow
artifact.

Important release details:

- Container tags do not have a leading `v`.
- Prerelease tags never move the `latest` image tag.
- Images use zstd compressed layers and OCI media types. Docker 23 or Podman
  4 is the minimum practical client version.
- No MSI is built.
- The macOS app and package are not code-signed.
- `NETBIRDY_ARTIFACT_KEY` must contain the artifact private key. The root
  private key stays on the maintainer machine.

## Published image checks

The `test-docker-compose` job in `test-infrastructure-files.yml` pulls the
published images only when the repository variable
`NETBIRDY_PUBLISHED_IMAGES` is `true`.
