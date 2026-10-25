# NetBirdy local preview with Podman

Everything here builds from source. The stack is two containers:

- `netbirdy-server` - combined server (management + signal + relay +
  STUN + embedded Dex IdP, sqlite storage) on http://localhost:8081,
  metrics on :9090, relay health on :9000, STUN on 3478/udp
- `netbirdy-dashboard` - the dashboard static export served by nginx
  on http://localhost:8080

## Usage

```sh
make podman-build   # build both images
make podman-up      # generate secrets/config, start detached
make podman-logs    # follow logs
make podman-test    # unit tests and builds inside containers
make podman-down    # stop the stack
make podman-clean   # stop and remove containers, volumes, images
```

`podman-up` runs `podman/setup.sh` first, which writes
`podman/config.yaml` (server config with generated secrets and a
seeded owner account) and `podman/.env` (owner credentials). Both are
gitignored. Re-run `podman/setup.sh --force` to regenerate.

The compose provider is `podman compose` (docker-compose against the
rootless podman socket). Override with `COMPOSE="podman-compose"` on
the make command line if needed.

## Notes

- HTTP only, localhost only. TLS and public exposure are out of scope
  for this preview.
- Anonymous metrics and geolite updates are disabled in the generated
  config (`disableAnonymousMetrics`, `disableGeoliteUpdate`).
- The OIDC client id `netbird-dashboard` is part of the embedded IdP
  configuration and is intentionally not renamed.
