# Crow CI

NetBirdy ships `.crow/` workflows so the project can be tested on a
self-hosted Crow server (Docker or Podman backend) without GitHub.

## Workflows

- repo-hygiene.yaml: shell syntax on tracked scripts, rebrand check,
  private key scan, govulncheck with the shared ignore list.
- go.yaml: build, vet, mod tidy, unit tests for management, shared,
  proxy, signal, relay and the updater. Depends on repo-hygiene.
- dashboard.yaml: pnpm install, lockfile check, docs:build, typecheck,
  lint, unit tests, build. Depends on repo-hygiene.
- podman-stack.yaml: manual only, builds the two Containerfiles.
  Requires an agent whose backend exposes a container socket to the
  step container.

## Connecting a repo

Create a forge OAuth app (GitHub forges, Forgejo, or Codeberg) for the
Crow server, add the repository in the Crow UI, and point the agent at
a Docker or Podman socket. No secrets are needed for the default
workflows; signing and publishing stay in GitHub Actions.

## Running locally

```sh
make crow-lint               # crow lint --strict .crow/
make crow-exec WORKFLOW=repo-hygiene.yaml
```

`crow exec` runs a workflow against a local Podman or Docker socket.
The workflows pin images by tag; digests belong in comments next to the
tag when pinned.
