# Security monitoring (anomaly detection)

NetBirdy management can detect suspicious IP changes and auth anomalies for
peers, agents and dashboard users, and optionally act on them.

## What is detected

- Peer source IP changes (new IP, subnet, ASN, country) vs a learned baseline
- Two concurrent sync connections for one peer key from different IPs
- Peer source IP flapping between many addresses
- Machine metadata changes for the same peer key (serial, OS, kernel)
- Setup key used from many IPs or with mass registrations
- Authentication failure bursts per source IP and per identity
- Successful auth after a failure burst (credential stuffing signal)
- Dashboard logins from a new country
- Personal access tokens used from a new IP

## Configuration

Management config `security:` section:

| Key | Default | Description |
|---|---|---|
| mode | off | off, observe or prevent; absent section = off |
| learning_period | 168h | baseline window before prevent may act |
| trusted_cidrs | [] | ranges that never trigger findings |
| log_file | <dataDir>/security-events.jsonl | structured event log |
| rules.<name>.enabled | true | per-rule toggle |
| rules.<name>.action | alert | prevent-mode action: require_approval, expire_login, block_peer |

The podman preview config enables `mode: observe` out of the box.

## Modes

- `observe` (recommended first): events go to the jsonl log and the activity
  audit feed; nothing is blocked; `would_prevent` marks rules that would act.
- `prevent`: after the learning period and per-account cap (5/hr), the
  configured rule action runs (require approval, expire login, block peer).
- `off`: the engine is not constructed; hooks never run.

New IPs always seed the baseline silently, so the first observation of a
peer is never flagged.

## Event log

One JSON object per line: `ts`, `type`, `severity` (info|low|medium|high),
`account_id`, `peer_id`, `peer_name`, `user_id`, `src_ip`, `previous_ips`,
`country`, `asn`, `rule`, `score` (0-100), `action_taken`, `mode`,
`would_prevent`, `reason`.

Findings also appear in the Activity audit feed as security event codes.

## False positives

Mobile networks, CGNAT and VPN exit changes produce new IPs. Trusted CIDRs,
the /24-subnet match and the 7-day learning period bound the noise; the
strongest signals are concurrent connections on one key and combined
new-IP+ASN+country changes.

## SIEM forwarding

`security.syslog` forwards security events as RFC 5424 (udp, tcp with
newline framing, or tcp+tls with ca_file/server_name; octet-counting via
`framing: octet`). `security.audit_log_file` mirrors every activity event
to a second jsonl (`audit-events.jsonl` in the podman template) with
sensitive meta keys (token, secret, password, key) stripped. The proxy
shield writes its own decision log via `--shield-log-file` /
`NB_PROXY_SHIELD_LOG_FILE` (non-pass decisions only: honeypot, ban,
challenge, tarpit).

## HTTP API

The security endpoints are admin or owner only and live under the regular
management API.

### GET /api/security/status

Returns the engine mode, learning period, per-rule state and the findings
counters accumulated since the engine started. When the `security:` section
is absent or `mode` is `off`, the engine is not running and `mode` reports
`"off"` with empty counters.

```json
{
  "mode": "observe",
  "learning_period": "168h0m0s",
  "rules": [{"name": "peer_new_ip", "enabled": true, "action": "alert"}],
  "findings_total": 12,
  "findings_by_rule": {"peer_new_ip": 12}
}
```

### GET /api/security/events?limit=200

Returns findings from the security event log, newest first. `limit`
defaults to 200 and is capped at 1000. Each event carries `time`, `type`,
`rule`, `severity`, `action`, the account/peer/user/src_ip it refers to,
`reason` and `would_prevent`.

### PUT /api/security/mode

Body `{"mode": "off" | "observe" | "prevent"}`.

When the engine is running, the mode switch applies immediately and the
response reports `{"mode": ..., "applied": true}` with a note that the
change is runtime only. When the engine is not running, the mode is written
into the `security:` section of the management config file and the response
reports `{"mode": ..., "applied": false, "note": "takes effect after
restart"}`. To persist a live change, update `security.mode` in the config
file as well.

## Reviewing and recovering

- Events appear in the Activity audit feed and `security-events.jsonl`.
- A flagged peer shows pending approval on the Peers page with an approve
  action; approving clears the flag.
- A peer whose login expired recovers by re-registering (new setup key or
  SSO), or an admin can clear expiry from the Peers page.
- Roll back prevention by setting `security.mode` to `observe` or `off`.

Note: `block_peer` is an alias of `expire_login` — management has no
separate peer block flag, the effect is the same forced re-auth refusal.

## Privacy

The log stores IPs and coarse geo only (no payload inspection). Rotate or
restrict `security-events.jsonl` like other server logs.
