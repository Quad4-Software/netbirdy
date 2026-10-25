# Supported protocols

| Protocol | Transport | Notes |
|---|---|---|
| HTTP/1.1 | TCP | default |
| HTTP/2 | TCP | ALPN h2, up to 250 concurrent streams |
| HTTP/3 | UDP | QUIC, same port as TLS, `--http3`/`NB_PROXY_HTTP3` (default on when TLS is on), `--http3-port`/`NB_PROXY_HTTP3_PORT` overrides |
| WebSocket | TCP | ws:// and wss:// via Upgrade hijacking; not supported over HTTP/3 (clients fall back to h1.1/h2, Alt-Svc is advisory) |
| SSE | TCP/UDP | events flush incrementally, not buffered |
| gRPC | TCP | unary and streaming over h2 |
| TCP passthrough | TCP | SNI-routed |
| UDP passthrough | UDP | SNI/session routed |
| PROXY protocol | TCP | v1/v2 inbound on listeners when `--proxy-protocol` |

HTTP/3 notes: certificates, SNI and ACME share the main TLS config, and ACME
tls-alpn-01 on TCP is unaffected. The QUIC handshake validates client
addresses with Retry tokens, so completed connections cannot be source-spoofed.
0-RTT is disabled to avoid replay risk. Firewall: allow UDP on the main port
in addition to TCP for HTTP/3 to be reachable; TCP-only setups keep working
over h1.1/h2.
