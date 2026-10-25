# Wazuh integration

How to feed NetBirdy security events into Wazuh and optionally let Wazuh
act back on flagged peers. Wazuh 4.x only; the 5.0 line changes decoders
and agents.

## Architecture

```
NetBirdy server --> security-events.jsonl --\
                   -> audit-events.jsonl    --> Wazuh agent or syslog
                   -> shield-events.jsonl --/        |
                                                manager -> rules -> alerts
                                                    |
                                            active response
                                                    |
                                        PUT /api/peers/{id} (PAT)
```

## What ships

`integrations/wazuh/` in the repo: decoders, rules (ids 100100-100303),
ossec snippets, the `netbirdy-respond` active response script, samples
and a test harness.

## Step 1: install the pack on the manager

Copy the decoder and rules files to `/var/ossec/etc/decoders/` and
`/var/ossec/etc/rules/` and restart wazuh-manager.

## Step 2: collect the logs

Preferred: a Wazuh agent on the NetBirdy host reading the three jsonl
files via `ossec/agent-localfile.conf`. The files live on the server's
data volume; find the mountpoint:

    podman volume inspect nby-data --format '{{.Mountpoint}}'

Alternative without an agent: enable `security.syslog` in the NetBirdy
config (udp, tcp or tcp+tls) and add `ossec/manager-syslog.conf` to the
manager so it listens on 514.

Peers can also run the Wazuh agent themselves; the overlay gives them a
direct path to the manager on its NetBirdy IP.

## Step 3: verify

Watch `security-events.jsonl` grow, then in the Wazuh dashboard check
alerts fire on the `netbirdy` rule group. `sh integrations/wazuh/test.sh`
validates the pack offline; `WAZUH_LOGTEST=1` runs the samples through
wazuh-logtest in a manager container.

## Response direction

The AR script marks flagged peers `approval_required` via
`PUT /api/peers/{id}` with a PAT. Start with `NETBIRDY_AR_MODE=dry-run`
in `/var/ossec/etc/netbirdy-ar.env`, review the AR log, then set
`enforce`. Never allowlist-check the account owner's own peers twice:
`NETBIRDY_ALLOWLIST` exempts peer ids/names. `delete` commands are a
no-op; undo approval from the Peers page.

## Deployment note

Do not run the Wazuh stack on the same podman host casually: the
official single-node deployment (wazuh/wazuh-docker) needs about 4 GB
RAM and several GB of images. Prefer agents against an existing manager.

## Uninstall

Remove the rules/decoders files, drop the localfile/ossec.conf blocks,
remove the AR script and env file, restart manager and agents.
