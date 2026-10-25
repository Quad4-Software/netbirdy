# Offline mode

When the management server is unreachable at startup, a NetBirdy client can
start from the last successfully applied network map instead of waiting for
the control plane.

## How it starts

Offline mode engages only when all of the following are true:

- the management server cannot be reached (dial, DNS or TLS failures; never
  on authentication or authorization rejections);
- a snapshot (`networkmap.bin` next to `config.json`) exists, parses, has the
  current schema version and is younger than the maximum age;
- the cached peer session has not expired.

While offline, the engine applies the cached map: the WireGuard interface,
peers, routes, DNS, firewall rules and SSH config come up as they were at
the last sync. The daemon keeps probing management with a jittered backoff
and returns to normal operation automatically when the server comes back.
A short tunnel flap is expected on reconnect.

## What keeps working

- Established peer connections whose endpoints are still reachable.
- DNS resolution for peers, routes, firewall rules, SSH config.

## What does not

- New peers, policy changes, new routes: the map is frozen.
- Relay-only peers once relay credentials expire.
- Anything that needs the signal service (new ICE negotiation).
- New logins or session changes.

## Security notes

- A cached map applies stale policy until expiry. `NB_OFFLINE_MAX_AGE`
  bounds that.
- The snapshot is deleted on logout, on a server-sent empty peer list, on
  session expiry, and on any authentication rejection observed while
  offline.
- The file is written `0600` next to `config.json`, same trust boundary as
  the WireGuard private key.

## Configuration

| Variable | Default | Effect |
| --- | --- | --- |
| `NB_OFFLINE_MAX_AGE` | `168h` | Snapshot validity window; `0` disables the feature entirely. |
| `NB_OFFLINE_START_DELAY` | `10s` | Wait before entering offline mode; `0` starts offline immediately after the first failed connect. |

## Checking the status

`netbirdy status` reports `Offline` for the connection state while cached
mode is active; the daemon logs enter and leave events at warn level.

## Testing with the podman stack

```
podman stop netbirdy-server
systemctl --user restart netbirdy-client   # or netbirdy down && netbirdy up
netbirdy status                            # shows Offline
podman start netbirdy-server
netbirdy status                            # returns to Connected
```

## Testing

`scripts/offline-e2e.sh` stands up a throwaway rig (`nbyoff-*` resources
only): a fixed-subnet network, a combined server, and two clients.
`up` builds the environment, `scenario <name>` runs one of running,
restart-one, restart-both, ip-change, server-back or revoke, `down`
removes everything it created. `make offline-e2e` runs the whole sweep.
Requires podman and /dev/net/tun access; a full run takes about 10
minutes. Logs land in /tmp/netbirdy-audit/offline/.
