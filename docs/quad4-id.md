# Quad4 ID

Sign in to NetBirdy with Quad4 ID (passkey-only OIDC, invites and optional
manual approval), optionally with user and group sync through a Quad4 ID
API key.

## Prerequisites

- A Quad4 ID instance reachable over HTTPS. The public hosted instance is
  https://auth.quad4.io; a self-hosted instance works the same way
  (podman profile `quad4` serves one at http://localhost:1412).
- In Quad4 ID: an OIDC client with the NetBirdy callback URL.
- Users join through invite links (Settings > Users > Invite); accounts
  held for manual approval cannot sign in until approved.

## 1. Create the OIDC client in Quad4 ID

- Callback / redirect URI: the exact value shown in the NetBirdy
  identity provider dialog, which is
  `<your NetBirdy base>/oauth2/callback`
  (for the podman preview: `http://localhost:8081/oauth2/callback`).
- Scopes: `openid`, `profile`, `email`, `groups`.
- Enable the `groups` claim for the client, otherwise group sync is
  empty.
- Note the client id and client secret.
- Optional: create an API key (Settings > API keys) for user/group sync.

## 2. Add it in NetBirdy

Dashboard: Settings > Identity Providers > Add > Quad4 ID.

- Issuer: your Quad4 ID URL with no trailing slash
  (`https://auth.quad4.io` for the public instance); it must be reachable
  from the management server and the `iss` claim must match.
- Client ID / Client secret: from the OIDC client.
- API key (optional): enables user and group sync.

Or via the helper: `podman/add-quad4.sh [url] <client-id> <secret> [api-key]`
creates the provider through the management API. The URL argument is
optional and defaults to `https://auth.quad4.io`.

Server-side user/group sync also works through `IdpManagerConfig` with
`ManagerType: "quad4"`, an `ApiToken`, and a `ManagementEndpoint` in
management.json.

## 3. Group mapping

Quad4 ID group names map to NetBirdy groups one to one; users joining a
matching group name are placed automatically on next sync.

## Troubleshooting

- Redirect mismatch: the callback URL in Quad4 ID must equal the shown
  value exactly (scheme, host, port, `/oauth2/callback`).
- Missing groups: enable the groups claim on the OIDC client and request
  the groups scope (the preset does).
- Pending approval: accounts created from an invite link that requires
  approval cannot sign in until an admin approves them in Quad4 ID.
- Issuer unreachable/mismatch: the management server must resolve the
  base URL; HTTPS is required outside localhost.
- Clock skew between Quad4 ID and management breaks token validation.

## Security notes

- The client secret and API key are stored in the management config/store;
  restrict API key scope to read-only user/group rights.
- Restrict who can register in Quad4 ID with single-use, expiring invite
  links, and keep manual approval enabled for untrusted audiences.
