# Event streaming

Streams account activity events to an external HTTP endpoint. Integrations are
managed at Dashboard > Integrations > Event Streaming or over the REST API at
`/api/integrations/event-streaming`. Configuration is stored per account in
`<datadir>/eventstream.json`.

Supported platforms on self-hosted installs:

- `generic_http` - POSTs each event to a configured URL.
- `datadog` - POSTs each event to the Datadog logs intake URL with the
  `DD-API-KEY` header.
- `s3`, `firehose` - rejected with `501 not supported`, since they need AWS
  request signing, which is not implemented on self-hosted.

At most one integration per account can be enabled. Enabling an integration
disables the others.

## generic_http

Config fields:

- `url` (required): delivery endpoint, `http` or `https`.
- `headers` (optional): JSON encoded object of request headers, for example
  `{"Authorization":"Bearer token"}`. Header names `authorization`,
  `proxy-authorization`, `x-api-key` and `dd-api-key` are treated as secrets:
  they are masked as `********` in API responses, and sending the mask or an
  empty value on update keeps the stored value.
- `body_template` (optional): Go template rendered over the event JSON fields.
  Empty sends the default event JSON.

Template example:

```
{"event": "{{ .activity_code }}", "target": "{{ .target_id }}", "meta_peer": "{{ index .meta "peer_id" }}"}
```

## datadog

Config fields:

- `api_url` (required): the logs intake URL of the Datadog site, for example
  `https://http-intake.logs.datadoghq.com/api/v2/logs`.
- `api_key` (required): sent as the `DD-API-KEY` request header. Masked in API
  responses. Sending `********` on update keeps the stored key.

## Event payload

Each activity event is POSTed as one JSON object:

```json
{
  "id": 123,
  "timestamp": "2026-01-02T03:04:05Z",
  "activity": "User created",
  "activity_code": "user.create",
  "activity_id": 91,
  "initiator_id": "user-1",
  "initiator_name": "Admin",
  "initiator_email": "admin@example.com",
  "target_id": "user-2",
  "account_id": "account-1",
  "meta": {"email": "user-2@example.com"}
}
```

Delivery retries a failed request up to 5 times with exponential backoff and
then drops the event. Events queue in a bounded in-memory buffer, so activity
recording is never blocked by a slow or unreachable endpoint.

## Example receiver

```bash
cat > /tmp/receiver.py <<'EOF'
from http.server import BaseHTTPRequestHandler, HTTPServer
import sys

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        print(dict(self.headers), file=sys.stderr)
        print(body.decode(), file=sys.stderr)
        self.send_response(200); self.end_headers()

HTTPServer(("127.0.0.1", 9999), H).serve_forever()
EOF
python3 /tmp/receiver.py
```

Then create the integration:

```bash
curl -X POST http://localhost:33071/api/integrations/event-streaming \
  -H "Authorization: Token <pat>" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "generic_http",
    "enabled": true,
    "config": {
      "url": "http://127.0.0.1:9999/events",
      "headers": "{\"X-Custom\":\"netbird\"}"
    }
  }'
```
