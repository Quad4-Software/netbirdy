#!/bin/sh
# Refresh proxy/internal/shield/data/ai-crawlers.txt from the ai.robots.txt
# project. Prints a diff summary. No dependencies beyond curl and coreutils.
set -eu
DST="$(dirname "$0")/../proxy/internal/shield/data/ai-crawlers.txt"
SRC="https://raw.githubusercontent.com/ai-robots-txt/ai.robots.txt/main/robots.json"
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
curl -fsSL "$SRC" | python3 -c '
import json,sys
d=json.load(sys.stdin)
print("# AI crawler user-agent tokens")
print("# source: '"$SRC"'")
print("# retrieved: " + __import__("datetime").date.today().isoformat())
for k in sorted(d.keys(), key=str.lower): print(k)
' > "$TMP"
diff "$DST" "$TMP" | grep -c '^[<>]' || true
mv "$TMP" "$DST"
echo "updated $DST"
