# Proxy shield control from management

The proxy shield blocks scanners, crawlers and abusive clients in front of
proxied services. `proxy/docs/shield.md` describes the detection engine,
signals and scoring. This page covers how management controls the shield
fleet-wide.

## How control reaches the proxies

Management keeps one global shield configuration in
`<dataDir>/shield-config.json` (mode 0600). It is delivered over the proxy
sync stream:

- `SyncMappingsResponse.shield_config` and
  `GetMappingUpdateResponse.shield_config` carry the config on every batch,
  so proxies converging after a reconnect always see the current value.
- `SyncMappingsResponse.shield_commands` carries imperative actions such as
  `unban_ip`. Commands for proxies that cannot take a live push stay queued
  and drain on their next sync-stream message.
- `SyncMappingsAck.shield_status` reports each proxy's mode, ban count,
  active tarpits, tracked sources and banned IPs back to management.

The proto fields are new and backward compatible: older proxies ignore them
and keep their local shield configuration, and older management servers
send nothing.

When no `shield-config.json` exists, the config is unmanaged: management
pushes nothing and every proxy keeps the shield settings from its own flags
and config file. Once a config is set over the API it is pushed on every
sync message and proxies apply it at runtime, replacing the pushed subset
of their local config. Proxies started with the shield disabled cannot be
enabled remotely because their request chain has no shield middleware; they
log and ignore the push.

## HTTP API

All endpoints require the admin or owner role.

### GET /api/proxy/shield

Returns the managed flag, the pushed config and the last reported status of
every connected proxy. `config` is null when unmanaged and a proxy's
`status` is null when it has not reported yet or uses the legacy stream.

```json
{
  "managed": true,
  "config": {
    "mode": "enforce",
    "allow_cidrs": ["10.0.0.0/8"],
    "ai_crawlers": "deny",
    "allow_search_engines": false,
    "allow_local": true
  },
  "proxies": [
    {
      "proxy_id": "edge-1",
      "status": {"mode": "enforce", "banned": 3, "active_tarpits": 0,
                 "tracked_sources": 512, "banned_ips": ["203.0.113.9"]}
    }
  ]
}
```

### PUT /api/proxy/shield

Body is the shield config: `mode` (`off`, `observe` or `enforce`),
`allow_cidrs`, `ai_crawlers` (`deny`, `allow` or `challenge`),
`allow_search_engines` and `allow_local`. The config is validated the same
way the proxy validates it, persisted to `shield-config.json` and pushed to
every connected proxy immediately. The response echoes the config and the
number of proxies pushed live.

### POST /api/proxy/shield/unban?ip=203.0.113.9

Queues an `unban_ip` command for every connected proxy and pushes it live
to sync-stream proxies. The response reports the IP and how many proxies
received the push immediately.

## Dashboard

The dashboard settings page reads `GET /api/proxy/shield` to render the
current mode, allow lists and per-proxy status, writes changes through
`PUT /api/proxy/shield` and exposes unban through
`POST /api/proxy/shield/unban`.
