#!/bin/sh
# Regenerate all NetBirdy raster brand assets from the SVG sources of truth.
# Usage: docs/brand/generate.sh [--check]
# Requires: rsvg-convert, ImageMagick (convert).
set -eu
cd "$(dirname "$0")/../.."
B=docs/brand
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
CHECK=0; [ "${1:-}" = "--check" ] && CHECK=1
W=$TMP; [ "$CHECK" -eq 0 ] && W=.

rsvg(){ rsvg-convert -w "$2" -h "$2" "$1" -o "$3"; }

# color variants used for tray state icons
sed 's/#d92a41/#808080/g' $B/mark.svg > $TMP/gray.svg
sed 's/#d92a41/#f59e0b/g' $B/mark.svg > $TMP/amber.svg
sed 's/#d92a41/#ef4444/g' $B/mark.svg > $TMP/red.svg
sed 's/#d92a41/#000000/g' $B/mark.svg > $TMP/black.svg
sed 's/#d92a41/#ffffff/g' $B/mark.svg > $TMP/white.svg

out(){ mkdir -p "$(dirname "$1")"; }

gen(){
  src=$1; size=$2; dst=$3
  out "$W/$dst"
  rsvg "$src" "$size" "$W/$dst"
}

# client tray icons (256px masters, state semantics preserved)
gen $B/mark.svg   256 client/ui/assets/netbirdy-systemtray-connected.png
gen $TMP/amber.svg 256 client/ui/assets/netbirdy-systemtray-connecting.png
gen $TMP/gray.svg  256 client/ui/assets/netbirdy-systemtray-disconnected.png
gen $TMP/red.svg   256 client/ui/assets/netbirdy-systemtray-error.png
gen $B/mark.svg   256 client/ui/assets/netbirdy.png
gen $B/mark.svg   256 client/ui/assets/netbirdy-menu.png
for s in 16 24; do gen $TMP/black.svg $s client/ui/assets/netbirdy-menu-$s.png; done
gen $TMP/black.svg 18 client/ui/assets/netbirdy-menu-about-18.png
# mono and macOS template variants (single black with alpha)
for f in client/ui/assets/netbirdy-systemtray-*-mono*.png client/ui/assets/netbirdy-systemtray-*-macos.png; do
  case $f in
    *connecting*) s=$TMP/amber.svg;; *error*) s=$TMP/red.svg;; *disconnected*) s=$TMP/black.svg;; *) s=$TMP/black.svg;;
  esac
  gen "$s" 256 "$f"
done
# dot variants: colored small mark at listed sizes
for f in client/ui/assets/netbirdy-menu-dot-*.png; do
  case $f in
    *connected*) s=$B/mark.svg;; *connecting*) s=$TMP/amber.svg;; *error*) s=$TMP/red.svg;; *) s=$TMP/gray.svg;;
  esac
  case $f in *-16.png) sz=16;; *-22.png) sz=22;; *) sz=32;; esac
  gen "$s" "$sz" "$f"
done
# menu svg
cp $B/mark.svg $W/client/ui/assets/svg/netbirdy-menu.svg 2>/dev/null || true
# wails/build icons
gen $B/mark.svg 512 client/ui/build/appicon.png 2>/dev/null || true
# social preview
out "$W/.github/social-preview.png"
convert -size 1280x640 xc:black \
  \( -background none -resize 200x200 $B/mark.svg -extent 280x280 -gravity center \) \
  -gravity west -geometry +120+0 -composite \
  -gravity west -fill '#f5f4f0' -font DejaVu-Sans-Bold -pointsize 96 -annotate +340+0 netbirdy \
  -gravity west -fill '#8a8a8a' -font DejaVu-Sans -pointsize 34 -annotate +344+90 'Private networking you run yourself' \
  "$W/.github/social-preview.png"

sed 's/#d92a41/#22c55e/g' $B/mark.svg > $TMP/green.svg
# tray: connected green
gen $TMP/green.svg 256 client/ui/assets/netbirdy-systemtray-connected.png
# favicons + app icons (crimson)
for s2 in 16 32 48; do gen $B/mark.svg $s2 /tmp-fav-$s2.png; done 2>/dev/null || true
mkdir -p $W/favicon-tmp $W/idp/dex/web/themes/light $W/proxy/web/src/assets $W/proxy/web/dist/assets
for s2 in 16 32 48; do rsvg $B/mark.svg $s2 $W/favicon-tmp/$s2.png; done
convert $W/favicon-tmp/16.png $W/favicon-tmp/32.png $W/favicon-tmp/48.png $W/idp/dex/web/themes/light/favicon.ico
gen $B/mark.svg 64 idp/dex/web/themes/light/favicon.png
gen $B/mark.svg 256 idp/dex/web/themes/light/logo.png
convert $W/favicon-tmp/16.png $W/favicon-tmp/32.png $W/favicon-tmp/48.png $W/proxy/web/src/assets/favicon.ico
convert $W/favicon-tmp/16.png $W/favicon-tmp/32.png $W/favicon-tmp/48.png $W/proxy/web/dist/assets/favicon.ico
gen $B/mark.svg 180 dashboard/public/apple-icon.png 2>/dev/null || true
gen $B/mark.svg 192 dashboard/public/icon-192.png 2>/dev/null || true
gen $B/mark.svg 512 dashboard/public/icon-512.png 2>/dev/null || true
convert -size 1280x640 xc:'#000000'   \( -background none -resize 200x200 $B/mark.svg -extent 280x280 -gravity center \)   -gravity west -geometry +120+0 -composite   -gravity west -fill '#e8e8e8' -font DejaVu-Sans-Bold -pointsize 96 -annotate +340+0 netbirdy   -gravity west -fill '#8a8a8a' -font DejaVu-Sans -pointsize 34 -annotate +344+90 'Private networking you run yourself'   $W/.github/social-preview.png
if [ "$CHECK" -eq 1 ]; then
  n=0
  (cd $W && find . -name '*.png' -o -name '*.svg' | while read -r f; do
    if [ -f ".$f" ]; then
      d=$(compare -metric AE -null: ".$f" "$W/$f" 2>&1 || true)
      [ "$d" != "0" ] && { echo "differs: $f"; n=$((n+1)); }
    else
      echo "new: $f"
    fi
  done)
  exit 0
fi
echo "generated"
