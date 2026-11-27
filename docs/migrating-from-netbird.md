# Migrating from NetBird

NetBirdy uses the same configuration paths, state paths, wire protocol, and
environment variables as NetBird. Most installs can be switched by replacing
the package or container image.

## Linux client packages

Install the NetBirdy `.deb` or `.rpm` over the existing NetBird package. The
package declares the needed replace and conflict metadata. The installer stops
the old `netbird` service, installs `netbirdy`, and leaves these paths in
place:

- `/etc/netbird`
- `/var/lib/netbird`

`/usr/bin/netbird` remains as a compatibility symlink. To roll back, install
the NetBird package again.

## Docker client

Replace the client image with `ghcr.io/quad4-software/netbirdy` and keep the
same environment variables and volumes. Both `NB_*` and `NETBIRD_*` variables
are still read.

## Self-hosted server

Replace the management, signal, relay, or combined server image with the
matching NetBirdy image. Keep the existing data directory and configuration.
Replace the dashboard image with `ghcr.io/quad4-software/netbirdy-dashboard`
and keep its environment variables.

The embedded identity provider configuration and the `netbird-dashboard` OIDC
client id are unchanged, so existing sign-in clients continue to work.

## Names that stay unchanged

These names are part of the compatibility surface:

- `/etc/netbird` and `/var/lib/netbird`
- `NB_*` and `NETBIRD_*` environment variables
- the `wt0` interface
- the `netbird-dashboard` OIDC client id
- `io.netbird.*` identifiers
- the Go module path `github.com/netbirdio/netbird`

Changing them would break existing data, policies, scripts, and client builds.

## Behavior differences

- Anonymous usage reporting is disabled.
- Version and update checks use this repository's releases, not NetBird's
  package services.
- GeoLite database downloads are opt in. Set the URL variables documented in
  the configuration before expecting country or ASN data.
- NetBird cloud service addresses remain compiled in for compatibility, but a
  self-hosted install should use its own management URL.

See [offline mode](offline-mode.md) for the client behavior added by this
fork.
