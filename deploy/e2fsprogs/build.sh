#!/bin/sh
# Builds mke2fs, debugfs and e2fsck from e2fsprogs's source to stand alone on a Mac, which has none: the three programs link only the
# system's libSystem, so one archive runs on a Mac with nothing installed. They are GPLv2 programs, so the archive carries the license
# and an offer of the source. The OpenShell driver runs them on the host to build each sandbox's disk.
#
#   deploy/e2fsprogs/build.sh dist     (on an Apple Silicon Mac, with the Xcode command line tools)
set -eu
out=${1:-dist}
version=1.47.4
sha256=fd5bf388cbdbe006a3d3b318d983b2948382440acc85a87f1e7d108653e8db0b
name=gusto-e2fsprogs-darwin-arm64.tar.gz
source=https://mirrors.edge.kernel.org/pub/linux/kernel/people/tytso/e2fsprogs/v$version/e2fsprogs-$version.tar.xz

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
out=$(cd "$out" && pwd)

curl -fsSL -o "$work/source.tar.xz" "$source"
echo "$sha256  $work/source.tar.xz" | shasum -a 256 -c -
tar -xf "$work/source.tar.xz" -C "$work"
cd "$work/e2fsprogs-$version"
CC=/usr/bin/clang ./configure --disable-nls --disable-elf-shlibs --disable-bsd-shlibs --disable-fuse2fs --disable-uuidd >/dev/null
make -j"$(sysctl -n hw.ncpu)" >/dev/null

pack="$work/pack"
mkdir "$pack"
cp misc/mke2fs debugfs/debugfs e2fsck/e2fsck "$pack/"
strip "$pack"/*
cp NOTICE "$pack/COPYING"
cat > "$pack/SOURCE.txt" <<TEXT
mke2fs, debugfs and e2fsck are e2fsprogs $version, built unchanged from its source with
  ./configure --disable-nls --disable-elf-shlibs --disable-bsd-shlibs --disable-fuse2fs --disable-uuidd

They are free software under the GNU General Public License version 2 (COPYING). The source is at
  $source
  sha256 $sha256
and the script that built these is deploy/e2fsprogs/build.sh in github.com/getlatentic/gusto.
TEXT
for tool in mke2fs debugfs e2fsck; do
  if otool -L "$pack/$tool" | tail -n +2 | grep -v "/usr/lib/libSystem" >/dev/null; then
    echo "$tool links more than the system's library" >&2
    exit 1
  fi
done
(cd "$pack" && tar -czf "$out/$name" mke2fs debugfs e2fsck COPYING SOURCE.txt)
printf '%s  %s  %s\n' "$(shasum -a 256 "$out/$name" | cut -d' ' -f1)" "$(stat -f %z "$out/$name")" "$name" > "$out/$name.sha256"
cat "$out/$name.sha256"
