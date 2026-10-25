---
title: Security Monitoring
description: Anomaly detection for suspicious IP changes and auth anomalies, with optional prevention.
order: 11
source: ../docs/security-monitoring.md
---
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

## Privacy

The log stores IPs and coarse geo only (no payload inspection). Rotate or
restrict `security-events.jsonl` like other server logs.
