# NetBirdy dashboard

This is the web interface for the NetBirdy management server. It is a Next.js
application vendored into this repository.

## What it does

The dashboard lets an operator manage peers, setup keys, users, groups,
routes, DNS, access control, identity providers, reverse proxy services, and
account settings.

## Running locally

The dashboard needs a reachable management API. For a full local system, use
the Podman stack from the repository root:

```sh
make podman-build
make podman-up
```

Then open <http://localhost:8080>.

For frontend-only development, install dependencies from the repository root
and start Next.js:

```sh
pnpm install --frozen-lockfile
pnpm --filter netbirdy-dashboard dev
```

Set the management API endpoint in the dashboard environment when it is not
the local default.

## Checks

```sh
pnpm --filter netbirdy-dashboard lint
pnpm --filter netbirdy-dashboard test:unit
npx tsc --noEmit
```

Visual and end-to-end test instructions are in `dashboard/AGENTS.md` and the
`e2e/` directory.

## Embedded documentation

The offline documentation shown in the dashboard lives in
`dashboard/content/docs/`. Pages marked as upstream documentation are vendored
reference material and have their own license notice.
