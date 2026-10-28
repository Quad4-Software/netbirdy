# Updating

## Podman preview stack

```sh
git pull
make podman-build   # rebuild images from current sources
make podman-down
make podman-up      # config and data persist in the volume
```

## Installed packages

Upgrade with the system package manager (`apt`, `dnf`, `zypper`). The
`netbirdy` packages declare `Replaces`/`Conflicts`/`Provides` on
`netbird`, so upgrading in place over a NetBird install keeps the same
config at `/etc/netbird`, the same `netbird` system user and the same
service file. See docs/migrating-from-netbird.md for the full matrix.

## Client auto-update

The client checks the management server version at login; upgrading
the server first is the recommended order. Auto-update of the client
binary itself is a per-distribution package-manager concern.

## Release verification

Every release asset ships with a reposign signature (`<file>.sig`) that
NetBirdy clients verify against the root key embedded in the binary, so
the mirror and the GitHub release page are both untrusted transports.

Verify manually:

```sh
gh attestation verify <file> --repo Quad4-Software/netbirdy
cosign verify-blob --signature checksums.sig \
  --certificate-identity-regexp 'https://github.com/Quad4-Software/netbirdy/.github/workflows/release-fork.yml.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  netbirdy_<ver>_checksums.txt
```

For container images, verify the digest in `netbirdy_<ver>_images.txt`
the same way, then pull by `image@sha256:<digest>`.

### Key rotation and revocation

Private keys live outside the repository in `~/.config/netbirdy-signing/`
(root.key and artifact.key, mode 600). The root key must stay offline
and backed up; the artifact key is also stored as the
`NETBIRDY_ARTIFACT_KEY` repository secret for CI signing.

Rotate the artifact key yearly:

```sh
go run ./client/cmd/signer create-artifact-key \
  --root-private-key-file ~/.config/netbirdy-signing/root.key \
  --artifact-priv-key-file ~/.config/netbirdy-signing/artifact.key \
  --artifact-pub-key-file ~/.config/netbirdy-signing/artifact-pub.pem \
  --expiration 8760h
go run ./client/cmd/signer bundle-pub-keys \
  --root-private-key-file ~/.config/netbirdy-signing/root.key \
  --artifact-pub-key-file ~/.config/netbirdy-signing/artifact-pub.pem \
  --bundle-pub-key-file publickeys/artifact-signatures/keys/artifact-key-pub.pem
```

To revoke a key:

```sh
go run ./client/cmd/signer extend-revocation-list \
  --key-id <key-id> \
  --revocation-list-file publickeys/artifact-signatures/keys/revocation-list.json \
  --private-root-key ~/.config/netbirdy-signing/root.key
```

## Server updates from the dashboard

The updater sidecar upgrades the Podman stack in place from the Settings
Updates page or the management API.

### What it does

verify the signed image list against the release root, pull each image
by digest, stop the server and dashboard in reverse order, back up the
data volume with a checksum manifest, recreate the containers from the
new images keeping labels, env, command and healthcheck, wait for
healthy, and on any failure roll back: remove the new containers,
rename the previous ones back, restore the data volume, start the old
set and mark the run rolled_back. The updater never updates itself.

### Enabling it

The updater ships as an optional compose profile:

    podman compose -f podman/compose.yaml --profile updater up -d

setup.sh generates NETBIRDY_UPDATER_TOKEN into podman/.env. The
management config can point at it via the commented server.updater
block (url http://netbirdy-updater:8089, token_env
NETBIRDY_UPDATER_TOKEN). The default socket is the rootless Podman
socket at ${XDG_RUNTIME_DIR}/podman/podman.sock; on a Docker host use
the Docker socket path.

### Security model

The socket mount gives the updater host-root equivalent for the user,
so treat the token as high value. The container publishes no ports and
only serves the management host. Only images under the configured
prefix and digests from the root signed list are applied. Backups live
in the netbirdy-updater-backups volume, pruned to the newest 5, and can
be restored by hand from the manifest. Status persists in
backups/status.json.

## AppImage

The desktop UI also ships as a thin AppImage for amd64. It needs the
netbirdy daemon package installed and running, and host GTK4 and
WebKitGTK 6.0 (Debian/Ubuntu: libgtk-4-1 libwebkitgtk-6.0-4, Fedora:
gtk4 webkitgtk6.0). chmod +x and run it; on hosts without FUSE it
runs with --appimage-extract-and-run.
