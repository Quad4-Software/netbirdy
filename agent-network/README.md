# Agent Network

Agent Network controls how AI agents reach model providers and internal
services. Each request carries a NetBirdy identity. Policies decide which
provider, model, and internal resource that identity may use.

Traffic stays inside the NetBirdy network until it reaches the reverse proxy.
The proxy checks identity and policy, injects the provider credential on the
server side, forwards the request, and records usage. Agents do not need a
copy of the provider API key.

## What it can do

- Set spending and rate limits for an agent, user, or group.
- Restrict providers and models.
- Record token use and cost by identity.
- Point agents at an existing AI gateway while keeping authentication in
  NetBirdy.
- Use the same identity rules for internal APIs and self-hosted models.

The feature is beta. It is self-hosted and does not require a hosted NetBirdy
service.

## Basic shape

An agent uses a private endpoint such as `https://mirror.example.net`. That
endpoint is served by the NetBirdy reverse proxy. The endpoint is reachable
only to users and agents allowed by network and identity policy.

The proxy then forwards approved requests to the configured model provider or
gateway. Internal resources can instead be reached directly over the overlay,
subject to the same access rules.

## Where the code lives

- `proxy/`: request handling, authentication, policy checks, limits, and
  provider forwarding.
- `management/internals/modules/agentnetwork/`: control-plane configuration,
  policies, guardrails, budgets, routing, and usage records.

## Roles

Agent Network permissions are split into submodules such as
`agent_network.providers`, `agent_network.policies`, `agent_network.budgets`,
and `agent_network.usage`. A role can grant one submodule or the parent area.

Two dedicated roles exist:

- `agent_network_admin`: manage Agent Network configuration and read the
  users, groups, peers, and account data needed for policies.
- `usage_viewer`: read usage data without changing configuration.

## Claude Code settings

Some Claude Code checks contact Anthropic directly instead of the configured
proxy endpoint. On a network without direct egress, set these when needed:

- `CLAUDE_CODE_SKIP_FAST_MODE_ORG_CHECK=1`
- `CLAUDE_CODE_SKIP_FAST_MODE_NETWORK_ERRORS=1`
- `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`

The exact settings depend on whether the endpoint serves `/v1/models` and
whether direct egress to `api.anthropic.com` is allowed.
