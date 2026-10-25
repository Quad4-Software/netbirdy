# NetBirdy Wazuh pack

Custom decoders, rules, ossec snippets, sample events and an active
response script so Wazuh ingests NetBirdy security, audit and proxy
shield events and can act back on flagged peers.

Wazuh 4.x only. Wazuh 5.0 (in beta, no in-place upgrade) replaces XML
decoders with YAML, CDB lists with KVDB and removes Filebeat; this pack
needs a rewrite for 5.0.

## Contents

- `decoders/netbirdy_decoders.xml` — decoder tags for our jsonl fields
- `rules/netbirdy_rules.xml` — rules 100100-100303 with MITRE mappings
- `ossec/agent-localfile.conf` — agent localfile blocks for the three
  jsonl files on the NetBirdy host
- `ossec/manager-syslog.conf` — manager remote syslog listener when
  using the SyslogSink variant instead of an agent
- `ossec/manager-active-response.conf` — command + active-response
  blocks for `netbirdy-respond`
- `active-response/netbirdy-respond` — AR script (python3 stdlib only)
  calling the NetBirdy API to mark a flagged peer approval-required
- `samples/` — one realistic event per rule + `expected.tsv` for logtest
- `test.sh` — XML lint, rule-id range check, AR unit tests, optional
  wazuh-logtest run

## Install on the manager

1. Copy `decoders/netbirdy_decoders.xml` to
   `/var/ossec/etc/decoders/netbirdy_decoders.xml`
2. Copy `rules/netbirdy_rules.xml` to
   `/var/ossec/etc/rules/netbirdy_rules.xml`
3. Merge `ossec/manager-active-response.conf` into ossec.conf (optional)
4. Copy `active-response/netbirdy-respond` to
   `/var/ossec/active-response/bin/` and chmod 750, owner root:wazuh
5. Create `/var/ossec/etc/netbirdy-ar.env` with NETBIRDY_URL and a PAT;
   start with `NETBIRDY_AR_MODE=dry-run`
6. `systemctl restart wazuh-manager`

## Install on the NetBirdy host

1. Enroll a Wazuh agent against your manager
2. Merge `ossec/agent-localfile.conf` into the agent ossec.conf and fix
   the jsonl paths (podman volume mountpoint:
   `podman volume inspect <vol> --format '{{.Mountpoint}}'`)
3. `systemctl restart wazuh-agent`

Or skip the agent and enable `security.syslog` in the NetBirdy config
pointing at the manager's remote syslog (see `ossec/manager-syslog.conf`).

## Validate

`sh test.sh` — XML well-formedness, unique in-range ids, AR unit tests.
`WAZUH_LOGTEST=1 sh test.sh` runs samples through wazuh-logtest in a
manager container.
