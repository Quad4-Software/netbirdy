# Migrating from NetBird

NetBirdy is a drop-in replacement: existing config, state and clients keep
working.

## Linux client (apt or rpm)

Installing the netbirdy package replaces netbird: the package declares
provides/replaces/conflicts, /usr/bin/netbird becomes a symlink to netbirdy,
and post_install.sh stops and removes the netbird systemd unit. Config in
/etc/netbird and state in /var/lib/netbird are untouched. Rollback: install
the upstream netbird package again; the same paths are used.

## Docker client

Swap the image to ghcr.io/quad4-software/netbirdy and keep the same env vars
(NB_*, NETBIRD_*) and volumes.

## Self-hosted server and dashboard

Swap the management/signal/relay (or combined) images to the netbirdy
equivalents and keep the same data directory and config.yaml. Swap the
dashboard image to netbirdy-dashboard with the same env. The OIDC client id
netbird-dashboard and the embedded IdP config carry over unchanged.

## Intentionally unchanged names

Paths /etc/netbird and /var/lib/netbird, env vars NB_* and NETBIRD_*, the wt0
interface, the netbird-dashboard OIDC client id, io.netbird.* ids, and the Go
module path github.com/netbirdio/netbird stay as NetBird so persisted data,
policies and tooling keep working.

## Differences to expect

- Anonymous metrics are off: the client metrics default endpoint is empty and
  the management selfhosted metrics endpoint is disabled unless a downstream
  build provides one.
- Version and self-update checks go to the fork GitHub releases
  (github.com/Quad4-Software/netbirdy), never to pkgs.netbird.io.
- The GeoLite geolocation database download is opt-in: set
  NB_GEOLOCATION_CITY_TAR_URL, NB_GEOLOCATION_CITY_TAR_SHA256_URL (and the
  _ZIP_ variants for CSV) to a source you choose.
- NetBird Cloud api.netbird.io/app.netbird.io remain the compiled defaults for
  cloud users; self-hosted installs configure them as before.

See also: [offline mode](offline-mode.md).
