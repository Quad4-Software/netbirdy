<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/wordmark-dark.svg" width="260">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/wordmark-light.svg" width="260">
    <img src="docs/brand/wordmark-light.svg" alt="NetBirdy" width="260">
  </picture>
</p>

# NetBirdy

NetBirdy is a self-hosted WireGuard mesh VPN. It is a fork of
[NetBird](https://github.com/netbirdio/netbird) and keeps the same wire
protocol, so existing clients and servers can be moved over without
re-enrolling every device.

[![CI](https://github.com/Quad4-Software/netbirdy/actions/workflows/golang-test-linux.yml/badge.svg)](https://github.com/Quad4-Software/netbirdy/actions/workflows/golang-test-linux.yml)

Website: <https://quad4-software.github.io/netbirdy/>

This is a personal fork for myself, friends, and family. LLMs were used while
building and maintaining it.

## What is different

- The dashboard has dark and light themes, a command palette, and offline
  documentation. Cloud-only pages are removed.
- Usage reporting is disabled. Update checks are opt in.
- The reverse proxy can slow or block scanners and crawlers. See
  [proxy/docs/shield.md](proxy/docs/shield.md).
- The reverse proxy supports HTTP/3, WebSocket, SSE, and gRPC.
- Clients can start from a cached network map while the management server is
  unreachable. See [docs/offline-mode.md](docs/offline-mode.md).
- Management can watch for suspicious peer IP changes, concurrent sessions,
  failed logins, and setup key misuse. See
  [docs/security-monitoring.md](docs/security-monitoring.md).
- Security events can be written as JSON lines, forwarded to syslog, or sent
  to Wazuh. See [docs/wazuh.md](docs/wazuh.md).
- Pocket ID and Quad4 ID have setup helpers in the dashboard and Podman stack.
- Releases are signed. The server can cache signed client updates for clients
  without direct access to GitHub. See [docs/updating.md](docs/updating.md)
  and [docs/update-mirror.md](docs/update-mirror.md).
- Linux, macOS, Windows, and FreeBSD client builds are published.
- Server processes can sandbox file system access with Landlock on Linux.
- A Podman stack is included for local use and testing.

## Quick start

You need Podman and make.

```sh
git clone https://github.com/Quad4-Software/netbirdy.git
cd netbirdy
make podman-build
make podman-up
```

The dashboard is at <http://localhost:8080>. The server is at
<http://localhost:8081>. The generated owner email and password are in
`podman/.env`.

Useful commands:

```sh
make podman-logs    # follow logs
make podman-down    # stop the stack
make podman-clean   # remove containers, volumes, and images
```

See [podman/README.md](podman/README.md) for ports and test commands.

## Install a client

Download a package or installer from the
[releases page](https://github.com/Quad4-Software/netbirdy/releases). Create a
setup key in the dashboard, then run:

```sh
sudo netbirdy up --management-url http://YOUR-SERVER:8081 --setup-key YOUR-KEY
netbirdy status
```

On Linux, the NetBirdy package replaces the NetBird package and keeps
`/etc/netbird` and `/var/lib/netbird`. See
[docs/migrating-from-netbird.md](docs/migrating-from-netbird.md).

## Compatibility

These names and formats intentionally stay compatible with NetBird:

| Item | Why |
| --- | --- |
| Go module path `github.com/netbirdio/netbird` | existing imports |
| `NB_*` and `NETBIRD_*` environment variables | existing configuration |
| `/etc/netbird`, `/var/lib/netbird`, the `netbird` service and user | in-place upgrades |
| Wire protocol, REST API, and setup keys | client and server interoperability |
| `X-Netbird-Fwd-*` headers | forwarded identity protocol |
| Package `Provides`, `Replaces`, and `Conflicts: netbird` | package migration |

Known gaps: NetBirdy does not publish phone apps, the desktop app is not
code-signed, and there is no Windows MSI. FreeBSD uses the userspace WireGuard
implementation and has no desktop app.

## Documentation

Start with these pages:

- [Migration from NetBird](docs/migrating-from-netbird.md)
- [Offline mode](docs/offline-mode.md)
- [Security monitoring](docs/security-monitoring.md)
- [Wazuh integration](docs/wazuh.md)
- [Updating](docs/updating.md)
- [Reverse proxy and shield](proxy/docs/)
- [Custom domains](docs/custom-domain-validation.md)
- [Fork maintenance](docs/maintaining.md)

The full index is [docs/README.md](docs/README.md).

## Repository layout

```text
client/        agent and desktop UI
management/    control plane and APIs
signal/        peer signaling
relay/         traffic relay
combined/      all-in-one server
dashboard/     web dashboard
proxy/         reverse proxy and shield
idp/           embedded identity provider
updater/       optional server updater
shared/        shared API types and update verification
podman/        local stack
integrations/  Wazuh pack
site/          project website
```

## Development

Run `tools/rebrand/rebrand.sh --check` after naming changes and
`docs/brand/generate.sh --check` after brand asset changes. Report security
problems as described in [SECURITY.md](SECURITY.md).

## License

Most of the repository is BSD-3-Clause. `management/`, `signal/`, `relay/`,
`combined/`, and `dashboard/` are AGPL-3.0. See [NOTICE.md](NOTICE.md) and
`LICENSES/`.

NetBird is a trademark of its owner. NetBirdy is an independent fork and is
not affiliated with or endorsed by the NetBird project.
