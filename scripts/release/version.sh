#!/bin/sh
# Derives a four part numeric version for NSIS APPVER and pkgbuild from a tag.
# v1.2.3 -> 1.2.3.0, v1.2.3-rc.1 -> 1.2.3.0, missing v prefix tolerated.
set -eu
v="${1#v}"
v="${v%%-*}" # strip prerelease
a="${v%%.*}"; rest="${v#*.}"
b="${rest%%.*}"; c="${rest##*.}"
printf '%s.%s.%s.0\n' "$a" "$b" "$c"
