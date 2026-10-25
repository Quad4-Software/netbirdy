#!/bin/sh
# Filter govulncheck JSON stream output against .github/govulncheck-ignore.txt.
# Usage: govulncheck-filter.sh <vuln-json-file>
# Exits 1 when an unlisted OSV id is found, 0 otherwise.
set -eu
exec python3 - "$1" <<'PYEOF'
import json, sys
ignore = {l.split("#")[0].strip() for l in open(".github/govulncheck-ignore.txt") if l.split("#")[0].strip()}
buf, depth, ids = "", 0, set()
for ch in open(sys.argv[1]).read():
    buf += ch
    depth += {"{":1,"}":-1}.get(ch, 0)
    if depth == 0 and buf.strip():
        obj = json.loads(buf)
        f = obj.get("finding")
        if f and f.get("osv"):
            ids.add(f["osv"])
        buf = ""
bad = sorted(ids - ignore)
if bad:
    print("unignored findings:", bad)
    sys.exit(1)
print("govulncheck clean (ignored:", sorted(ignore), ")")
PYEOF
