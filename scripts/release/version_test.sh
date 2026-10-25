#!/bin/sh
set -eu
chk() { [ "$(sh scripts/release/version.sh "$1")" = "$2" ] || { echo "FAIL $1 -> want $2"; exit 1; }; }
chk v1.2.3 1.2.3.0
chk v1.2.3-rc.1 1.2.3.0
chk 0.9.10 0.9.10.0
echo PASS
