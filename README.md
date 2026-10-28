<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/wordmark-dark.svg" width="260">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/wordmark-light.svg" width="260">
    <img src="docs/brand/wordmark-light.svg" alt="NetBirdy" width="260">
  </picture>
</p>

# NetBirdy

NetBirdy is a self-hosted WireGuard mesh VPN. It is a fork of
[NetBird](https://github.com/netbirdio/netbird) that uses the same protocol, so
NetBird clients can connect to a NetBirdy server and an existing NetBird setup
can be moved over without re-enrolling devices.

[![CI](https://github.com/Quad4-Software/netbirdy/actions/workflows/golang-test-linux.yml/badge.svg)](https://github.com/Quad4-Software/netbirdy/actions/workflows/golang-test-linux.yml)

Website: <https://quad4-software.github.io/netbirdy/>

> **Note:** LLMs were used to create and maintain this fork. It is a personal
> fork for myself, friends and family only.

## Differences from NetBird

- Dashboard with a black dark theme and a light theme, a command palette, and
  built-in documentation that works offline. Cloud-only and license-gated
  pages are removed.
- Usage reports to the NetBird project are disabled and update checks are opt-in.
- Reverse proxy shield that detects scanners and crawlers by behavior and
  slows or blocks them. See [proxy/docs/shield.md](proxy/docs/shield.md).
- HTTP/3, WebSocket, SSE and gRPC (unary) support in the reverse proxy.
- Clients cache the network map and management addresses so they can reconnect
  to peers after a restart while the server is unreachable. See
  [docs/offline-mode.md](docs/offline-mode.md).
- Anomaly detection for peer IP changes, concurrent connections, failed logins
  and setup key misuse, with off, observe and prevent modes. See
  [docs/security-monitoring.md](docs/security-monitoring.md).
- JSON audit and security logs, an optional syslog sink, and a Wazuh 4.x rule
  pack. See [docs/wazuh.md](docs/wazuh.md).
- Pocket ID and Quad4 ID setup helpers in the dashboard, compose profiles
  and scripts. See [docs/pocket-id.md](docs/pocket-id.md) and
  [docs/quad4-id.md](docs/quad4-id.md).
- Signed releases, a server-side cache for client updates, and an optional
  updater container that backs up and rolls back. See
  [docs/updating.md](docs/updating.md) and [docs/update-mirror.md](docs/update-mirror.md).
- Landlock sandbox: server processes limit themselves to the paths they need.
  See [docs/landlock.md](docs/landlock.md).
- Podman stack for local use and testing.

## Quick start

The local stack needs Podman and make.

```sh
git clone https://github.com/Quad4-Software/netbirdy.git
cd netbirdy
make podman-build
make podman-up
```

The dashboard is at <http://localhost:8080> and the server at
<http://localhost:8081>. The owner email and password are written to
`podman/.env`. Other targets:

```sh
make podman-logs    # follow logs
make podman-down    # stop the stack
make podman-clean   # stop and remove containers, volumes and images
```

See [podman/README.md](podman/README.md) for ports, resetting and tests.

## Install the client

Packages and installers are published on the
[releases page](https://github.com/Quad4-Software/netbirdy/releases). The binary is
named `netbirdy`.

Create a setup key in the dashboard, then on the device:

```sh
sudo netbirdy up --management-url http://YOUR-SERVER:8081 --setup-key YOUR-KEY
netbirdy status
```

Installing the NetBirdy Linux package replaces the NetBird package and keeps
`/etc/netbird` and `/var/lib/netbird`. See
[docs/migrating-from-netbird.md](docs/migrating-from-netbird.md).

## Compatibility

These stay the same as upstream so existing setups keep working:

| Item | Reason |
| --- | --- |
| Go module path `github.com/netbirdio/netbird` | existing imports |
| `NB_*` environment variables | existing configuration |
| `/etc/netbird`, `/var/lib/netbird`, the `netbird` service and user | in-place upgrades |
| Wire protocol, REST API, setup keys | interoperability with NetBird clients |
| `X-NetBird-Fwd-*` headers | forwarded identity protocol |
| Package `Provides`, `Replaces` and `Conflicts: netbird` | package migration |

Known gaps: NetBirdy does not ship its own phone apps (use the NetBird apps),
the desktop app is not code-signed, and there is no Windows MSI.

## Documentation

| Document | Contents |
| --- | --- |
| [docs/migrating-from-netbird.md](docs/migrating-from-netbird.md) | Moving from NetBird |
| [docs/offline-mode.md](docs/offline-mode.md) | Client offline mode and its limits |
| [docs/security-monitoring.md](docs/security-monitoring.md) | Anomaly detection, audit log, syslog |
| [docs/wazuh.md](docs/wazuh.md) | Wazuh integration |
| [docs/pocket-id.md](docs/pocket-id.md) | Pocket ID sign-in |
| [docs/quad4-id.md](docs/quad4-id.md) | Quad4 ID sign-in |
| [docs/updating.md](docs/updating.md) | Updating servers, packages and clients |
| [docs/update-mirror.md](docs/update-mirror.md) | Client update cache on the server |
| [docs/landlock.md](docs/landlock.md) | Landlock sandbox for server processes |
| [proxy/docs/](proxy/docs/) | Reverse proxy, shield, protocol support |
| [docs/custom-domain-validation.md](docs/custom-domain-validation.md) | Custom domains |
| [docs/maintaining.md](docs/maintaining.md) | Releases and fork maintenance |
| [docs/ci-crow.md](docs/ci-crow.md) | Crow CI pipelines |

The full index is [docs/README.md](docs/README.md).

## Repository layout

```
client/         agent (netbirdy) and desktop UI sources
management/     control plane, REST and gRPC APIs
signal/         peer signaling
relay/          traffic relay
combined/       all-in-one server binary
dashboard/      web dashboard (Next.js)
proxy/          reverse proxy and shield
idp/            embedded identity provider (Dex)
updater/        optional server updater
shared/         shared API types and update verification
podman/         local stack
integrations/   Wazuh pack
site/           project website (GitHub Pages)
```

## Development

Run `tools/rebrand/rebrand.sh --check` after naming changes and
`docs/brand/generate.sh --check` after brand asset changes. Report security
problems as described in [SECURITY.md](SECURITY.md). This is a personal project,
so issues and pull requests may not get a quick answer.

## License

BSD-3-Clause, except `management/`, `signal/`, `relay/`, `combined/` and
`dashboard/`, which are AGPL-3.0. See [NOTICE.md](NOTICE.md) and `LICENSES/` for
the license texts, the list of changes from NetBird and the AGPL source offer.

NetBird is a trademark of its owner. NetBirdy is an independent fork and is not
affiliated with or endorsed by the upstream project.
