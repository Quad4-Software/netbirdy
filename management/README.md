# NetBirdy management server

The management server is the control plane. It stores accounts, peers, users,
groups, policies, routes, DNS settings, and setup keys. Clients connect to it
for network maps, and the dashboard uses its REST API.

For normal use, run the combined server through the Podman stack or a release
container rather than running this component by hand.

## Local build

```sh
go build ./management
```

## Configuration

The server reads `management.json`. A template is available at
`infrastructure_files/management.json.tmpl`. Important settings include the
data directory, listening addresses, identity provider configuration, signal
and relay endpoints, and TLS settings.

## Container

The release image is `ghcr.io/quad4-software/netbirdy-management`, but most
installations should use `ghcr.io/quad4-software/netbirdy-server`, which
combines management, signal, and relay in one process.

Persist the data directory, usually `/var/lib/netbird`, when running any
server container.

## Development

Regenerate protobuf files with the scripts under `shared/management/proto/`
and `client/proto/`. Do not edit generated files directly.
