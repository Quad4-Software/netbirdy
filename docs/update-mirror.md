# Update mirror

The management server can cache signed client update artifacts fetched
from GitHub releases and serve them to NetBirdy clients over the
management HTTPS port. The mirror is an untrusted transport: every
file is verified against the embedded reposign root and the release
checksums before it is admitted to the cache, and clients verify again
independently.

## Configuration

`server.update_mirror` in the combined config, or `update_mirror:` in
the management config. Disabled by default; upgrades change nothing.

- enabled: false by default.
- repository: defaults to `Quad4-Software/netbirdy`.
- api_base_url: testing only, defaults to `https://api.github.com`.
- cache_dir: defaults to `<datadir>/updates`.
- max_cache_bytes: 2 GiB default, LRU evicted, target version kept.
- poll_interval: 1h default with jitter.
- github_token_env: name of an env var holding an optional API token
  for rate limits.
- platforms: explicit `os/arch` list, or `auto` (default) derived from
  the GoOS and platform of registered peers. No store schema change.
- prefetch: when true and the account target is `disabled`, latest is
  still cached. Default false.

## Endpoints (unauthenticated)

- `GET /updates/v1/manifest.json` mirror status.
- `GET /updates/v1/files/v<ver>/<name>` cached files, Range and
  If-Range supported.
- `GET /updates/v1/tag/v<ver>/<name>.sig` signatures.
- `GET /updates/v1/keys/<name>` reposign public key tree (fetched and
  verified against the embedded root at runtime, so rotations need no
  server release).

The keys layout is what reposign expects, so
`<management-url>/updates/v1` works as a keys base URL.

## Security model

Files are admitted only after a sha256 check against the release
checksums file and a reposign signature verification. Names and
versions pass strict allowlist regexes, per-IP rate limiting and
bounded concurrent transfers apply, and only cached files are served
with `nosniff`. HTTPS only to github.com and githubusercontent hosts.

## Limits

The mirror serves Windows and macOS installers only; Linux clients
keep using distribution packages. Management is not a WireGuard peer,
so peer-to-peer update relay over the tunnel is not part of this.
Air-gapped clients use the mirror as a drop-in keys base URL.
