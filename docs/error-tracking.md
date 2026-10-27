# Error tracking

The NetBirdy services can report panics and captured errors to a
Sentry-compatible endpoint. This covers sentry.io as well as self-hosted
GlitchTip and Bugsink, which speak the same protocol and use the same DSN
format.

Reporting is strictly opt-in. With no DSN configured the services do not
initialize a client and every reporting path is a no-op.

## Enabling per service

Each service reads its DSN from its own environment variable, so a single
deployment can route events from different services to different projects.

| Service                   | Environment variable    | Flag          |
| ------------------------- | ----------------------- | ------------- |
| management                | `NB_SENTRY_DSN`         | none          |
| signal                    | `NB_SIGNAL_SENTRY_DSN`  | none          |
| relay                     | `NB_RELAY_SENTRY_DSN`   | none          |
| proxy                     | `NB_PROXY_SENTRY_DSN`   | `--sentry-dsn` |
| combined `netbirdy-server` | `NB_SENTRY_DSN`         | none          |

Example:

```bash
NB_SENTRY_DSN="https://public@glitchtip.example.com/1" netbirdy-mgmt management
```

For the proxy the flag is equivalent to the environment variable:

```bash
proxy --sentry-dsn "https://public@bugsink.example.com/2"
```

A malformed DSN is logged as a warning at startup and reporting stays
disabled; it does not stop the service.

## Agent daemon and desktop app

The client daemon takes the same DSN through `--sentry-dsn` on the `service`
subcommands or the `NB_SENTRY_DSN` environment variable:

```bash
netbirdy service install --sentry-dsn "https://public@glitchtip.example.com/1"
```

The flag is stored in `service.json` like the other install parameters, so a
later `netbirdy service install` or `reconfigure` keeps it unless the flag is
passed again. `NB_SENTRY_DSN` can also be registered on the service directly
with `--service-env`.

The desktop app reads `NB_SENTRY_DSN` from its own environment, or the
`--sentry-dsn` flag:

```bash
NB_SENTRY_DSN="https://public@glitchtip.example.com/1" netbirdy-ui
```

The Go side of the app initializes reporting with the same helper as the
daemon. The React frontend learns the DSN from the GUI process through the
bound `Observability` service and initializes `@sentry/browser` with it, so a
single setting covers both sides of the desktop app. There is no separate
daemon-to-GUI plumbing: the DSN must reach the GUI process through its
environment or flags.

Events from the daemon are tagged with service `netbirdy`, events from the
desktop app with `netbirdy-ui`.

## What is sent

- Panics that reach the service entrypoint. The panic is reported, then
  re-panicked so the process keeps its original crash behavior.
- Errors captured through `observability.CaptureError`, including recovered
  panics in the proxy middleware dispatcher and shield wrapper.
- Each event carries the service name (as server name and `service` tag),
  the build version as the release, and the environment.

The environment tag defaults to `production` and can be overridden with
`NB_ENVIRONMENT`, for example `NB_ENVIRONMENT=staging`.

## Notes

- Events are buffered asynchronously and flushed with a 2 second timeout on
  shutdown or panic, so reporting adds no latency to request paths.
- Do not point a DSN at an endpoint you do not control or trust: captured
  errors can contain hostnames, file paths, and other runtime details.
