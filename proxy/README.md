# NetBirdy reverse proxy

The reverse proxy is a public entry point for services inside a NetBirdy
network. Management sends route configuration to the proxy. When a request
arrives, the proxy authenticates the user if the service requires it, then
forwards the traffic to the peer hosting the service.

Supported authentication methods are:

- none
- OIDC
- emailed magic link
- PIN
- HTTP basic authentication

## Management connection

The proxy connects to management over gRPC. It receives service updates on a
server-side stream and sends access logs on a client-side stream.

Machine-to-machine OAuth credentials authenticate the proxy to management.
Create those credentials in the configured identity provider and pass them to
the proxy with flags or environment variables.

## Request handling

The proxy selects a service from the request host. If the service has no
authentication method, it is public. Otherwise the proxy presents the allowed
sign-in methods. A successful sign-in does not guarantee that the backend is
reachable. Peer connectivity and the service itself can still fail.

### Target access control

HTTP targets have an optional `access_action` setting under **Edit Target,
Optional Settings, Access** in the dashboard.

| Value | Behavior |
| --- | --- |
| `inherit` | Use the service authentication setting. This is the default. |
| `bypass` | Forward without prompting for service authentication. |
| `block` | Return HTTP 403, including for authenticated users. |

The longest matching location prefix selects the target. Prefixes are literal
and case-sensitive. `/api` also matches `/apiv2`. `/api/` only matches paths
beginning with `/api/` and does not match `/api` itself. Wildcards and regular
expressions are not supported.

A blocked request or failed authentication never falls through to another
target. Deleting or disabling a target removes its rule, so a less-specific
target may then match.

Bypass does not disable service IP, country, or reputation restrictions. It is
not available for private services or Agent Network targets. Bypassed requests
have no NetBirdy user identity. Header-auth credentials, session cookies, and
client-supplied identity headers are removed before forwarding.

Services using target access overrides reject ambiguous paths, including
encoded separators and traversal segments, before forwarding.

Upgrade all proxies before using target access actions. Older proxies ignore
these actions and continue applying the service authentication setting.

## TLS

The proxy serves HTTPS even when the backend service uses plain HTTP.
Certificates can come from ACME or from `tls.crt` and `tls.key` files in the
certificate directory. Let's Encrypt is the default ACME provider, but another
ACME directory can be configured.

## Authentication UI

The authentication UI is a Vite and React application in `web/`. It is
embedded in the Go binary.

Build it from the repository root:

```sh
pnpm install --frozen-lockfile
pnpm --filter web build
```

For development with hot reload:

```sh
pnpm --filter web dev
```

The development server listens on <http://localhost:3031>.

## Configuration

Flags override environment variables.

| Flag | Environment variable | Purpose | Default |
| --- | --- | --- | --- |
| `-debug` | `NB_PROXY_DEBUG_LOGS` | enable debug logging | `false` |
| `-mgmt` | `NB_PROXY_MANAGEMENT_ADDRESS` | management server address | `https://api.netbird.io:443` |
| `-addr` | `NB_PROXY_ADDRESS` | reverse proxy listen address | `:443` |
| `-url` | `NB_PROXY_URL` | public proxy URL | `proxy.netbird.io` |
| `-cert-dir` | `NB_PROXY_CERTIFICATE_DIRECTORY` | certificate storage directory | `./certs` |
| `-acme-certs` | `NB_PROXY_ACME_CERTIFICATES` | use ACME certificates | `false` |
| `-acme-addr` | `NB_PROXY_ACME_ADDRESS` | HTTP-01 challenge listen address | `:80` |
| `-acme-dir` | `NB_PROXY_ACME_DIRECTORY` | ACME directory URL | Let's Encrypt production |
| `-oidc-id` | `NB_PROXY_OIDC_CLIENT_ID` | OIDC client id | `netbirdy-proxy` |
| `-oidc-secret` | `NB_PROXY_OIDC_CLIENT_SECRET` | OIDC client secret | empty |
| `-oidc-endpoint` | `NB_PROXY_OIDC_ENDPOINT` | OIDC provider endpoint | `https://api.netbird.io/oauth2` |
| `-oidc-scopes` | `NB_PROXY_OIDC_SCOPES` | comma-separated OIDC scopes | `openid,profile,email` |

## Protocols

The proxy supports HTTP/1.1, HTTP/2, HTTP/3, WebSocket, SSE, gRPC, and TCP or
UDP passthrough. See [docs/protocols.md](docs/protocols.md). HTTP/3 uses UDP
on the same port as TLS unless `--http3-port` overrides it.
