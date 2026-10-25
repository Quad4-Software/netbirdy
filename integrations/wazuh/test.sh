#!/bin/sh
# Validate the NetBirdy Wazuh pack: XML well-formed, unique in-range ids,
# AR unit tests, optional wazuh-logtest run when WAZUH_LOGTEST=1 against a
# running nby-wazuh-test container with the pack mounted.
set -eu
cd "$(dirname "$0")"

for f in decoders/netbirdy_decoders.xml rules/netbirdy_rules.xml; do
    if command -v xmllint >/dev/null 2>&1; then
        (echo '<r>'; cat "$f"; echo '</r>') | xmllint --noout -
    else
        python3 -c "import xml.dom.minidom; xml.dom.minidom.parseString('<r>'+open('$f').read()+'</r>')"
    fi
done
echo "XML OK"

python3 - <<'PY'
import re, sys
ids = re.findall(r'<rule id="(\d+)"', open('rules/netbirdy_rules.xml').read())
bad = [i for i in ids if not (100000 <= int(i) <= 120000)]
dup = {i for i in ids if ids.count(i) > 1}
if bad:
    sys.exit(f"rule ids out of range: {bad}")
if dup:
    sys.exit(f"duplicate rule ids: {dup}")
print(f"ids OK ({len(ids)} rules)")
PY

python3 -m unittest discover -s active-response -p 'test_*.py' 2>&1 | tail -3

if [ "${WAZUH_LOGTEST:-0}" = "1" ]; then
    fails=0; total=0
    while read -r f id lvl; do
        [ -z "$f" ] && continue
        total=$((total+1))
        name=${f%.json}
        podman exec nby-wazuh-test sh -c "cat /tmp/samples/$name.json | timeout 15 /var/ossec/bin/wazuh-logtest -U \"$id:$lvl:json\"" 2>&1 | grep -q "Unit test OK" || { echo "FAIL $name expected=$id/$lvl"; fails=$((fails+1)); }
    done < samples/expected.tsv
    echo "logtest: $((total-fails))/$total pass"
    [ "$fails" = "0" ]
fi
echo "DONE"
