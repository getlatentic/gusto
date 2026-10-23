#!/bin/sh
# Rebuilds basecoat.css from the npm package: the structure of the components gusto uses, then the vega style pack.
# gusto already has colours called muted (text) and accent (brand), so the package's background colours of those names are
# renamed bc-muted and bc-accent here, and defined that way in ../../basecoat-theme.css.
# Usage: update.sh [version]   (needs npm; commit the diff)
set -eu
here=$(cd "$(dirname "$0")" && pwd)
version=${1:-1.1.0}
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
(cd "$scratch" && npm pack "basecoat-css@$version" >/dev/null && tar xzf basecoat-css-*.tgz)
dist="$scratch/package/dist"
{
  printf '%s\n' "/* Basecoat $version (MIT, https://github.com/hunvreus/basecoat): the structure of the components gusto uses, then the vega style pack."
  printf '%s\n' "   Not taken from the package: its :root colours, @theme, base layer and dark variant, which gusto's tokens provide (see"
  printf '%s\n' "   ../../basecoat-theme.css), and the structure of the components gusto does not use. Regenerate with update.sh. */"
  for component in button input native-select checkbox field label textarea; do
    cat "$dist/components/$component.css"
    printf '\n'
  done
  cat "$dist/styles/vega.css"
} | sed -E 's/\bbg-muted\b/bg-bc-muted/g; s/\bbg-accent\b/bg-bc-accent/g; s/\btext-accent-foreground\b/text-bc-accent-foreground/g' > "$here/basecoat.css"
cp "$scratch/package/LICENSE.md" "$here/LICENSE.md"
