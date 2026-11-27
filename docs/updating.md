# Updating

## Podman preview stack

```sh
git pull
make podman-build
make podman-down
make podman-up
```

Configuration and data remain in the Podman volumes.

## Linux packages

Upgrade with the system package manager. The NetBirdy package replaces the
NetBird package and keeps the same configuration, state directory, system
user, and service compatibility names. See
[migrating-from-netbird.md](migrating-from-netbird.md).

## Client updates

Update the server before updating clients. Clients check the server version
when they log in. Package installation and update automation are handled by
the operating system or by the update mirror described in
[update-mirror.md](update-mirror.md).

## Verifying release files

Release files have reposign signatures in matching `.sig` files. The checksum
manifest also has a GitHub artifact attestation and a Cosign bundle. Container
images carry SBOM and provenance attestations.

Verify a downloaded file:

```sh
gh attestation verify <file> --repo Quad4-Software/netbirdy
cosign verify-blob --bundle netbirdy_<ver>_checksums.txt.cosign.bundle \
  --certificate-identity-regexp 'https://github.com/Quad4-Software/netbirdy/.github/workflows/release-fork.yml.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  netbirdy_<ver>_checksums.txt
```

`netbirdy_<ver>_images.txt` lists the legacy updater images.
`netbirdy_<ver>_images_provenance.txt` lists every published image variant.
Verify an image before pulling its digest:

```sh
cosign verify \
  --certificate-identity-regexp 'https://github.com/Quad4-Software/netbirdy/.github/workflows/release-fork.yml.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/quad4-software/netbirdy-server@sha256:<digest>
```

The image SBOM is attached to the OCI index. Inspect it with:

```sh
docker buildx imagetools inspect <image>@sha256:<digest>
```

OpenVEX statements are published only after a product-specific assessment
supports the status claim.

## Signing key rotation

Private keys belong in `~/.config/netbirdy-signing/` with mode 600. The root
key stays offline and backed up. The artifact key is also stored as the
`NETBIRDY_ARTIFACT_KEY` repository secret for release signing.

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

Revoke a key:

```sh
go run ./client/cmd/signer extend-revocation-list \
  --key-id <key-id> \
  --revocation-list-file publickeys/artifact-signatures/keys/revocation-list.json \
  --private-root-key ~/.config/netbirdy-signing/root.key
```

## Server updates from the dashboard

The optional updater sidecar can upgrade the Podman server stack from the
Settings Updates page or the management API.

The updater verifies the signed image list, pulls images by digest, backs up
the data volume with a checksum manifest, recreates the server and dashboard
containers, waits for health checks, and rolls back on failure. It does not
update itself.

Enable it with the updater compose profile:

```sh
podman compose -f podman/compose.yaml --profile updater up -d
```

`podman/setup.sh` writes `NETBIRDY_UPDATER_TOKEN` to `podman/.env`. The
management configuration can then use the commented `server.updater` block.
The default socket is the rootless Podman socket at
`${XDG_RUNTIME_DIR}/podman/podman.sock`.

The socket mount gives the updater broad control over containers for that
user. Treat the token as a high-value secret. The updater publishes no ports
and only accepts images below the configured prefix with digests from the
signed image list. Backups are stored in the `netbirdy-updater-backups`
volume and pruned to the newest five.

## AppImage

The desktop UI is also published as a thin amd64 AppImage. It requires the
`netbirdy` daemon package, GTK4, and WebKitGTK 6.0 on the host. Make the file
executable and run it. On hosts without FUSE, run it with
`--appimage-extract-and-run`.
