package openshell_test

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeOpenshell is an openshell command that keeps each sandbox as a directory,
// so the driver's whole sequence can be run without a gateway. exec runs the
// command in that directory with only the environment it was given, which is
// what a sandbox does.
const fakeOpenshell = `#!/bin/bash
root=${OPENSHELL_FAKE_ROOT:?}
echo "$*" >> "$root/calls.log"
while [ "${1:-}" = "--gateway" ]; do shift 2; done
cmd=$1; sub=${2:-}
case "$cmd $sub" in
"sandbox create")
  shift 2; name=""; policy=""
  while [ $# -gt 0 ]; do case "$1" in
    --name) name=$2; shift 2;; --policy) policy=$2; shift 2;; --) break;; *) shift;; esac; done
  mkdir -p "$root/$name/sandbox/workspace"; cp "$policy" "$root/$name/policy.json"; echo "Created sandbox: $name";;
"sandbox upload")
  mkdir -p "$root/$3$5"; cp -R "$4" "$root/$3$5/";;
"sandbox exec")
  shift 2; name=""; workdir=/; envs=()
  while [ $# -gt 0 ]; do case "$1" in
    --name|-n) name=$2; shift 2;; --workdir) workdir=$2; shift 2;; --env) envs+=("$2"); shift 2;;
    --no-login-shell|--no-tty|--no-credential-warnings) shift;; --) shift; break;; *) shift;; esac; done
  # A path the caller names under the sandbox's home is under that sandbox here.
  mapped=(); for argument in "$@"; do case "$argument" in /sandbox|/sandbox/*) argument="$root/$name$argument";; esac; mapped+=("$argument"); done
  cd "$root/$name$workdir" && exec env -i PATH="$PATH" "${envs[@]}" "${mapped[@]}";;
"sandbox download")
  dest=$5; mkdir -p "$dest"; cp -R "$root/$3$4/." "$dest/";;
"sandbox ssh-config") printf 'Host openshell-%s.default\n    User sandbox\n' "$3";;
"sandbox delete") rm -rf "$root/$3"; echo "$3" >> "$root/deleted.log";;
"logs "*|"logs")
  # A log that has not all arrived the first time it is asked for.
  n=$(cat "$root/logs.count" 2>/dev/null || echo 0); echo $((n + 1)) > "$root/logs.count"
  printf '%s\n' '{"event":"net.allow","host":"host.openshell.internal","port":8041}'
  [ "$n" -ge 1 ] && printf '%s\n' '{"event":"net.deny","host":"example.com","port":443}'
  true;;
"policy get") cat "$root/$3/policy.json";;
*) echo "fake openshell: unexpected $*" >&2; exit 2;;
esac
`

// fakeSSH is the ssh command that reaches a fake sandbox: it runs what it is
// asked to run in that sandbox's directory, with the sandbox's home mapped.
const fakeSSH = `#!/bin/bash
root=${OPENSHELL_FAKE_ROOT:?}
while [ $# -gt 0 ]; do case "$1" in -F|-o) shift 2;; -T) shift;; *) break;; esac; done
name=${1#openshell-}; name=${name%.default}
remote=${2//\/sandbox/$root/$name/sandbox}
echo "ssh $name" >> "$root/calls.log"
exec env -i PATH="$PATH" bash -c "$remote"
`

// installFake puts the fake first on PATH and returns where its sandboxes are.
func installFake(t *testing.T) string {
	t.Helper()
	bin, root := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "openshell"), []byte(fakeOpenshell), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(fakeSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSHELL_FAKE_ROOT", root)
	return root
}
