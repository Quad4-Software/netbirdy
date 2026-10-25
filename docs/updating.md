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
gh attestation verify <file> --repo Sudo-Ivan/netbirdy
cosign verify-blob --signature checksums.sig \
  --certificate-identity-regexp 'https://github.com/Sudo-Ivan/netbirdy/.github/workflows/release-fork.yml.*' \
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
