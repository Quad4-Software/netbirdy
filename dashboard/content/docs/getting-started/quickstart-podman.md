---
title: Quickstart with Podman
description: Bring up the whole stack locally with rootless Podman in a few minutes.
order: 20
---

The repository ships a rootless-friendly Podman preview under `podman/`.
It builds a combined server, a dashboard and an embedded identity provider.

## Requirements

- Podman 4 or newer with podman-compose or the compose plugin
- `openssl` and `go` on the host (the setup script generates secrets)

## Bring the stack up

```bash
cd podman
./setup.sh        # generates .env and config.yaml
podman compose up -d --build
```

When it is running:

- Dashboard: `http://localhost:8080`
- Combined server: `http://localhost:8081`
- Embedded IdP issuer: `http://localhost:8081/oauth2`

> **Note** The owner account and generated secrets are written to
> `podman/.env`. Keep that file private.

## Logs and teardown

```bash
podman compose logs -f server
podman compose down
```

The management data lives in a named volume, so `down` does not wipe
accounts or peers. To start over completely, remove the volumes listed in
`podman/compose.yaml`.
