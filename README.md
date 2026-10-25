<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/wordmark-dark.svg" width="260">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/wordmark-light.svg" width="260">
    <img src="docs/brand/wordmark-light.svg" alt="NetBirdy" width="260">
  </picture>
</p>

# NetBirdy

Self-hostable WireGuard overlay network with a privacy-first control
plane. NetBirdy is a fork of NetBird: same protocol, same client
compatibility, hardened defaults, extra security tooling, and a
restyled dashboard.

[![CI](https://github.com/Sudo-Ivan/netbirdy/actions/workflows/golang-test-linux.yml/badge.svg)](https://github.com/Sudo-Ivan/netbirdy/actions/workflows/golang-test-linux.yml)

## What is different from upstream

| Feature | Where documented |
| --- | --- |
| Crimson monochrome dashboard: black dark theme, warm paper light theme, Instrument Sans and JetBrains Mono vendored locally with no third-party requests | `docs/brand/` |
| Command palette and keyboard shortcuts | dashboard UI, `docs/brand/` |
| Embedded, offline-searchable documentation inside the dashboard | `dashboard/content/docs/` |
| Enterprise, cloud-only and license-gated UI removed | `NOTICE.md`, dashboard |
| OG and PWA metadata for dashboard pages | `dashboard/` |
| Reverse proxy scanner and crawler shield: honeypot paths, scanner and AI crawler UA classification, scoring, proof-of-work challenge, tarpit, bans, JSONL shield log, debug routes | `proxy/docs/shield.md` |
| HTTP/3 with Alt-Svc, plus a documented protocol support matrix: HTTP/1.1, HTTP/2, WS/WSS, SSE, gRPC unary, TCP/UDP passthrough, PROXY protocol, gaps documented (no h2c backends, no WebSocket over HTTP/3) | `proxy/docs/` |
| Listener hardening and trusted-proxy IP resolution | `proxy/docs/` |
| Offline-capable client: cached network map and cached management IPs, with documented limits | `docs/offline-mode.md` |
| Anomaly detection engine with off / observe / prevent modes: rules, learning period, hourly caps, cooldowns, owner safeguards | `docs/security-monitoring.md` |
| JSONL audit and security event logs, optional RFC 5424 syslog sink (UDP/TCP/TLS) | `docs/security-monitoring.md` |
| Wazuh 4.x integration pack: decoders, rules 100100-100303, dry-run-by-default active response (Wazuh 5.0 unsupported) | `docs/wazuh.md`, `integrations/wazuh/` |
| Pocket ID login: guided dashboard preset, compose profile, `podman/add-pocketid.sh` helper | `docs/pocket-id.md` |
| One-command Podman preview stack, telemetry off by default | `podman/README.md` |
| Drop-in compatibility: Go module path, `NB_*` env vars, `/etc/netbird` paths, wire protocol, package provide/replace for `netbird`, stock NetBird clients (including the mobile apps) work against a NetBirdy server | `docs/migrating-from-netbird.md` |
| Supply-chain defaults: pnpm 12 workspace with 7-day release-age cooldown, lockfile guard, SHA-pinned GitHub Actions, Go 1.27 | `docs/frontend-supply-chain.md` |

## Quick start (Podman preview)

```sh
make podman-build   # build server and dashboard images
make podman-up      # generate secrets and config, start detached
make podman-logs    # follow logs
make podman-down    # stop the stack
```

- Dashboard: `http://localhost:8080`
- Management / embedded IdP / signal / relay / STUN: `http://localhost:8081`
- Owner credentials are generated into `podman/.env` on first run.

See `podman/README.md` for the full matrix (custom ports, reset,
testing inside the containers).

## Install clients

Client packages and binaries are named `netbirdy` (`netbirdy`,
`netbirdy-ui`, `netbirdy-signal`, `netbirdy-relay`). The official NetBird mobile apps work against a NetBirdy server.

## Compatibility

| Kept as upstream | Why |
| --- | --- |
| Go module path `github.com/netbirdio/netbird` | import compatibility |
| `NB_*` environment variables | existing configs keep working |
| `/etc/netbird` config path and `netbird` service/user | drop-in upgrades |
| Wire protocol, API, setup keys | interop with stock clients |
| `x-netbird-fwd-*` protocol headers | forwarded-identity protocol |
| packages `Provides`/`Replaces`/`Conflicts: netbird` | package-level migration |
| `docs.netbird.io`, `pkgs.netbird.io` references where no fork equivalent exists | upstream links must keep working |

## Security

Report vulnerabilities privately: see `SECURITY.md`. Hardening notes:
`docs/security-monitoring.md`, `proxy/docs/shield.md`, `docs/wazuh.md`.

## Documentation

| File | Contents |
| --- | --- |
| `docs/migrating-from-netbird.md` | Migrating an existing NetBird install |
| `docs/offline-mode.md` | Client offline mode behaviour and limits |
| `docs/security-monitoring.md` | Anomaly detection, audit log, syslog |
| `docs/wazuh.md` | Wazuh SIEM integration |
| `docs/pocket-id.md` | Pocket ID sign-in setup |
| `docs/updating.md` | Updating server, packages and clients |
| `docs/maintaining.md` | Fork maintenance and release process |
| `docs/frontend-supply-chain.md` | Frontend dependency policies |
| `docs/testing-privileged.md` | Privileged test setup |
| `docs/custom-domain-validation.md` | Custom domains on the reverse proxy |
| `podman/README.md` | Podman preview stack reference |
| `proxy/docs/` | Reverse proxy, shield, protocol matrix |
| `integrations/wazuh/README.md` | Wazuh pack install |

## Updating

See `docs/updating.md`.

## Project layout

```
client/         agent (CLI netbirdy), runs on each machine
management/     control plane, REST + gRPC API
signal/         peer handshake broker
relay/          traffic relay for peers that cannot connect directly
combined/       all-in-one server binary
dashboard/      management web UI (Next.js)
proxy/          reverse proxy with the scanner/crawler shield
idp/            embedded Dex identity provider
shared/         shared proto and API types
podman/         local preview stack (Containerfiles, compose, setup)
integrations/   third-party integrations (Wazuh)
client/ui/      desktop UI (Wails)
```

## Contributing

Issues and pull requests go to
[github.com/Sudo-Ivan/netbirdy](https://github.com/Sudo-Ivan/netbirdy).
Run `tools/rebrand/rebrand.sh --check` before pushing identifier
changes and `docs/brand/generate.sh --check` for brand assets.

## License

BSD-3-Clause except `management/`, `signal/`, `relay/`, `combined/` and
`dashboard/`, which are AGPL-3.0-only. See `NOTICE.md` and
`LICENSES/` for the full license texts and the change notice.

## Trademark

NetBird is a trademark of its respective owner; see `NOTICE.md`.
NetBirdy is an independent fork and is not affiliated with or endorsed
by the upstream project.
