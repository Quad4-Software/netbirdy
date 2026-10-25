# NetBirdy Shield

proxy/internal/shield sits in front of proxied services and applies a
decaying per-source reputation score. It blocks scanners, abusive crawlers and
probe traffic while keeping false positives low. It is not a WAF and it is
not DDoS protection.

## Modes

off scores nothing, observe scores and logs without altering responses,
enforce applies actions.

## Actions by score band

| Band | Action |
| --- | --- |
| below challenge_at | pass |
| challenge_at (40) | proof-of-work interstitial, browser navigations only |
| tarpit_at (70) | delayed 403 after tarpit_delay, capped by max_tarpits |
| ban_at (100) | 403, escalating TTL ban (15m, x4, cap 24h) |

## Signals and default weights

| Signal | Weight |
| --- | --- |
| honeypot/probe path (only when the service does not serve it) | 100 |
| scanner user agent | 80 |
| research scanner UA | 20 |
| empty UA | 10 |
| fake or inconsistent browser UA | 25 / 6 |
| probe rate over window | 40 |
| enumeration of distinct paths | 40 |
| unknown host or IP-literal Host | 15 |
| odd method/proto/JNDI/shell metachars | 15 |
| sustained rate over limit | 50 |
| unverified search-engine claim | 90 |
| TLS-level signal via Observe | 30 |
| auth failure via Observe | 30 |

Honeypot hits count only when the request hits an unknown host or the service
answered 400/403/404/405, so a service really exposing /wp-login.php stays
fine.

## AI crawlers and search engines

The AI crawler token list ships from ai.robots.txt
(proxy/internal/shield/data/ai-crawlers.txt, refreshed by
scripts/update-shield-lists.sh). Default policy deny. Search engines are
denied unless allow_search_engines is set; verification needs a resolver wired
in by the integrator, and a failed claim is a strong signal.

## Challenge

GET navigations that claim a browser UA get a small SHA-256 PoW page bound to
IP, UA hash, host and expiry, signed with an HMAC key (generated at startup or
from hmac_key for multi-replica). Solving sets an HttpOnly pass cookie.

## Privacy

IPs are kept in memory only, keyed per /64 for IPv6, capped at max_entries.
An optional state file persists active bans. Nothing leaves the proxy.

## Prior art

- TecharoHQ Anubis design doc (SHA-256 PoW interstitial concept only).
- CrowdSec hub scenarios http-probing, http-sensitive-files,
  http-bad-user-agent, http-crawl-non_statics (signal shapes and thresholds).
- OWASP Bot Management cheat sheet (verification of claimed good bots).
- ai-robots-txt project (crawler UA data file).

## Wiring (4d)

Flags: --shield-mode, --shield-config, --shield-state-file,
--shield-allow-cidrs, --shield-allow-search-engines, --shield-ai-crawlers
(env NB_PROXY_SHIELD_*). The chain order is hijack -> meter -> accessLog ->
shield -> assets -> auth -> proxy, so the client IP comes from CapturedData.

Search engine verification uses forward-confirmed reverse DNS (PTR suffix
plus forward match). Allowed suffixes per vendor docs:
googlebot.com/google.com (Google), search.msn.com (Bing), crawl.baidu.com
/.jp (Baidu), yandex.com/.net/.ru (Yandex), duckduckgo.com (DuckDuckGo),
applebot.apple.com (Apple). Cache 6h positive, 10m negative, 2s timeout.

TLS signals (SNI missing, IP literal, unknown) are observed in
GetCertificate; they never affect the handshake. With PROXY protocol
enabled the connection remote address already carries the real client IP;
without it, the load balancer address is observed instead.

Debug endpoints (with --debug-endpoint): GET /debug/shield/stats,
GET /debug/shield/bans, POST path /debug/shield/unban?ip=...
Metrics: netbirdy_proxy_shield_decisions_total{action,reason_class},
gauges netbirdy_proxy_shield_banned and _tarpits.
