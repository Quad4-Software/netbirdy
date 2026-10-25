<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/wordmark-dark.svg" width="260">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/wordmark-light.svg" width="260">
    <img src="docs/brand/wordmark-light.svg" alt="NetBirdy" width="260">
  </picture>
</p>

# NetBirdy

NetBirdy is a WireGuard-based overlay network with a self-hosted
control plane. It is a fork of NetBird and the NetBird dashboard.

## What is in this repository

- `client/` - the agent (CLI `netbirdy`), runs on each machine
- `management/` - control plane and REST/gRPC API
- `signal/` - peer handshake broker
- `relay/` - traffic relay for peers that cannot connect directly
- `combined/` - all-in-one server (management + signal + relay + STUN
  + embedded IdP, binary `netbirdy-server`)
- `dashboard/` - the web management UI (Next.js static export served
  by nginx)
- `proxy/` - identity-aware reverse proxy
- `podman/` - local preview stack (server + dashboard)

## Quick start (Podman)

Requires podman and a compose provider (podman-compose or
docker-compose against the podman socket).

```sh
make podman-build   # build the server and dashboard images from source
make podman-up      # generate config/secrets and start the stack
```

Then open http://localhost:8080 for the dashboard. The server listens
on http://localhost:8081. `make podman-logs`, `make podman-down`, and
`make podman-clean` manage the stack. See podman/README.md for details.

## Build

```sh
go build ./client        # netbirdy agent CLI
go build ./combined      # netbirdy-server all-in-one binary
cd dashboard && npm ci && npm run build   # dashboard static export
```

`make build-dashboard` runs the dashboard build, `make test-dashboard`
runs its unit tests and lint, and `make podman-test` runs a fast test
pass inside containers.

## Theme

The dashboard ships a dark modern black theme by default and a light
paper-and-verdigris theme, switchable from the header theme toggle.

## License and attribution

NetBirdy is a modified fork of NetBird
(https://github.com/netbirdio/netbird) and the NetBird dashboard
(https://github.com/netbirdio/dashboard). See NOTICE.md for the change
summary and license map. Everything is BSD-3-Clause except
`management/`, `signal/`, `relay/`, `combined/`, and `dashboard/`,
which are AGPL-3.0-only. Contributions to this fork are under the
respective directory licenses.

NetBird is a trademark of NetBird GmbH. NetBirdy is not affiliated
with or endorsed by NetBird GmbH.

For running without the control plane see [docs/offline-mode.md](docs/offline-mode.md).
