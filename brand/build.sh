#!/usr/bin/env bash
# Draws every icon and image the product ships from brand/gusto-mark.svg. Run it after changing the mark.
# Needs rsvg-convert and magick. The macOS icon set needs iconutil, so that file is only made on a Mac.
set -euo pipefail
cd "$(dirname "$0")/.."
mark=brand/gusto-mark.svg
mono=brand/gusto-mark-mono.svg
web=internal/web/static
icons=desktop/src-tauri/icons
desk=internal/helper/desk

# The mark on a square canvas with a margin, as a fraction of the canvas.
padded() { # size margin-percent out [svg]
  local size=$1 pct=$2 out=$3 svg=${4:-$mark} inner
  inner=$(( size * (100 - 2 * pct) / 100 ))
  rsvg-convert -w "$inner" -h "$inner" --keep-aspect-ratio "$svg" -o "$out.inner.png"
  magick -size "${size}x${size}" xc:none "$out.inner.png" -gravity center -composite "$out"
  rm -f "$out.inner.png"
}

cp "$mark" "$web/mark.svg"
rsvg-convert -w 192 --keep-aspect-ratio "$mark" -o "$web/mark.png"

# The browser tab icon and the touch icon: the badge fills its square, on the page colour for the touch icon.
rsvg-convert -w 32 -h 32 --keep-aspect-ratio "$mark" -o "$web/favicon-32.png"
padded 180 10 "$web/apple-touch-icon.png"
magick "$web/apple-touch-icon.png" -background '#F5FBF6' -alpha remove -alpha off "$web/apple-touch-icon.png"

# The desktop app. The g is a hole in the badge, and an app icon has a desktop behind it, so the hole is filled with the page colour.
# The badge is convex, so its convex hull is the badge without the g.
padded 1024 10 /tmp/gusto-icon-cut.png
hull=$(magick /tmp/gusto-icon-cut.png -channel A -separate +channel -threshold 50% -format "%[convex-hull]" info: | tr -d '\n')
magick -size 1024x1024 xc:none -fill '#F5FBF6' -draw "polygon $hull" /tmp/gusto-icon-fill.png
magick /tmp/gusto-icon-fill.png /tmp/gusto-icon-cut.png -composite /tmp/gusto-icon-1024.png
magick /tmp/gusto-icon-1024.png -resize 512x512 "$icons/icon.png"
magick /tmp/gusto-icon-1024.png -resize 256x256 "$icons/128x128@2x.png"
magick /tmp/gusto-icon-1024.png -resize 128x128 "$icons/128x128.png"
magick /tmp/gusto-icon-1024.png -resize 32x32 "$icons/32x32.png"
sizes=()
for s in 16 24 32 48 64 256; do magick /tmp/gusto-icon-1024.png -resize "${s}x${s}" "/tmp/gusto-ico-$s.png"; sizes+=("/tmp/gusto-ico-$s.png"); done
magick "${sizes[@]}" "$icons/icon.ico"
if command -v iconutil >/dev/null; then
  set_dir=$(mktemp -d)/gusto.iconset; mkdir -p "$set_dir"
  for s in 16 32 128 256 512; do
    magick /tmp/gusto-icon-1024.png -resize "${s}x${s}" "$set_dir/icon_${s}x${s}.png"
    magick /tmp/gusto-icon-1024.png -resize "$((s * 2))x$((s * 2))" "$set_dir/icon_${s}x${s}@2x.png"
  done
  iconutil -c icns "$set_dir" -o "$icons/icon.icns"
fi

# The menu bar icon is a template: one colour and transparent, so the system tints it.
padded 44 6 "$icons/tray.png" "$mono"
magick "$icons/tray.png" -fill black -colorize 100 "$icons/tray.png"

# The helper's tray icon.
cp "$icons/tray.png" "$desk/icon.png"
magick "$desk/icon.png" "$desk/icon.ico"

echo "icons written"
