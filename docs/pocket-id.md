# Pocket ID

Sign in to NetBirdy with Pocket ID (passkeys) over OIDC, optionally with
user and group sync through a Pocket ID API key.

## Prerequisites

- A Pocket ID instance reachable over HTTPS (or localhost for testing).
  First-run admin setup happens in the Pocket ID browser UI with a
  passkey.
- In Pocket ID: an OIDC client with the NetBirdy callback URL.

## 1. Create the OIDC client in Pocket ID

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

Dashboard: Settings > Identity Providers > Add > PocketID.

- Issuer: your Pocket ID URL with no trailing slash
  (for example `https://id.example.com`); it must be reachable from the
  management server and the `iss` claim must match.
- Client ID / Client secret: from the OIDC client.
- API key (optional): enables user and group sync.

Or via the helper: `podman/add-pocketid.sh <url> <client-id> <secret> [api-key]`
creates the provider through the management API.

## 3. Group mapping

Pocket ID group names map to NetBirdy groups one to one; users joining a
matching group name are placed automatically on next sync.

## Troubleshooting

- Redirect mismatch: the callback URL in Pocket ID must equal the shown
  value exactly (scheme, host, port, `/oauth2/callback`).
- Missing groups: enable the groups claim on the OIDC client and request
  the groups scope (the preset does).
- Issuer unreachable/mismatch: the management server must resolve the
  base URL; HTTPS is required outside localhost.
- Clock skew between Pocket ID and management breaks token validation.

## Security notes

- The client secret and API key are stored in the management config/store;
  restrict API key scope to read-only user/group rights.
- Restrict which Pocket ID user groups may use NetBirdy in the Pocket ID
  client settings.
