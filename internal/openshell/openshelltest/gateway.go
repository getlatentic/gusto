// Package openshelltest is a stand-in OpenShell gateway for tests: the openshell command, run on this computer, keeping each
// sandbox as a directory.
package openshelltest

import (
	"os"
	"path/filepath"
	"testing"
)

// gateway keeps each sandbox as a directory, enough for a job to run in one.
const gateway = `#!/bin/bash
root=${OPENSHELL_FAKE_ROOT:?}
echo "$*" >> "$root/calls.log"
while [ "${1:-}" = "--gateway" ]; do shift 2; done
cmd=$1; sub=${2:-}
case "$cmd $sub" in
"sandbox create") shift 2; name=""; while [ $# -gt 0 ]; do case "$1" in --name) name=$2; shift 2;; --) break;; *) shift;; esac; done
  mkdir -p "$root/$name/sandbox"; echo "Created sandbox: $name";;
"sandbox get") [ -d "$root/$3" ] || { echo "Error: sandbox not found" >&2; exit 1; };;
"sandbox start") rm -f "$root/$3/stopped";;
"sandbox stop") touch "$root/$3/stopped";;
"sandbox upload") mkdir -p "$root/$3$5"; cp -R "$4" "$root/$3$5/";;
"sandbox exec") shift 2; name=""; workdir=/; while [ $# -gt 0 ]; do case "$1" in --name) name=$2; shift 2;; --workdir) workdir=$2; shift 2;;
    --no-login-shell|--no-tty) shift;; --) shift; break;; *) shift;; esac; done
  [ -e "$root/$name/stopped" ] && { echo "Error: sandbox is stopped" >&2; exit 1; }
  mapped=(); for a in "$@"; do case "$a" in /sandbox|/sandbox/*) a="$root/$name$a";; esac; mapped+=("$a"); done
  cd "$root/$name$workdir" && exec env -i PATH="$PATH" "${mapped[@]}";;
"sandbox download") mkdir -p "$5"; cp -R "$root/$3$4/." "$5/";;
"sandbox delete") rm -rf "$root/$3"; echo "$3" >> "$root/deleted.log";;
"service expose") echo "✓ Exposed service $5 on sandbox $3 -> 127.0.0.1:$4"; echo "  URL: http://default--$3--$5.openshell.localhost:${OPENSHELL_FAKE_GATEWAY_PORT:-18080}/"; echo "$3 $4 $5" >> "$root/exposed.log";;
"service delete") echo "$3 $4" >> "$root/unexposed.log";;
"logs "*|"logs") echo '{"event":"net.allow"}';;
"policy get") echo "policy";;
*) echo "fake gateway: unexpected $*" >&2; exit 2;;
esac
`

// Install puts the stand-in first on the PATH for the rest of the test, and returns the directory its sandboxes are kept in.
// calls.log there has every command it was given, and deleted.log every sandbox it deleted.
func Install(t testing.TB) string {
	t.Helper()
	bin, root := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "openshell"), []byte(gateway), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSHELL_FAKE_ROOT", root)
	return root
}
