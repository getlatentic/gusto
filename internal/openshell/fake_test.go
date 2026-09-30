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
  # A gateway that is staging as many archives as it will refuses the next ones, and takes them a moment later.
  if [ -s "$root/busy" ] && [ "$(cat "$root/busy")" -gt 0 ]; then
    echo $(( $(cat "$root/busy") - 1 )) > "$root/busy"; echo "Error:   × failed to allocate a rootfs tar staging slot on the gateway" >&2; exit 1
  fi
  if [ -e "$root/broken" ]; then echo "Error:   × the gateway cannot make this sandbox" >&2; exit 1; fi
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
  [ -e "$root/$name/stopped" ] && { echo "Error:   × sandbox is stopped" >&2; exit 1; }
  cd "$root/$name$workdir" && exec env -i PATH="$PATH" "${envs[@]}" "${mapped[@]}";;
"sandbox download")
  dest=$5; mkdir -p "$dest"; cp -R "$root/$3$4/." "$dest/";;
"service expose")
  # The address a real gateway gives, on the port a test says its gateway is on.
  echo "✓ Exposed service $5 on sandbox $3 -> 127.0.0.1:$4"; echo "  URL: http://default--$3--$5.openshell.localhost:${OPENSHELL_FAKE_GATEWAY_PORT:-18080}/"; echo "$3 $4 $5" >> "$root/exposed.log";;
"service delete") echo "$3 $4" >> "$root/unexposed.log"; grep -q "^$3 .* $4$" "$root/exposed.log" 2>/dev/null || { echo "Error:   × service not found" >&2; exit 1; };;
"sandbox ssh-config") printf 'Host openshell-%s.default\n    User sandbox\n' "$3";;
"sandbox get") [ -d "$root/$3" ] && echo "Name: $3" || { echo "Error:   × sandbox not found" >&2; exit 1; };;
"sandbox start") touch "$root/starts.log"; echo "$3" >> "$root/starts.log"; rm -f "$root/$3/stopped";;
"sandbox stop") touch "$root/$3/stopped";;
"sandbox delete") rm -rf "$root/$3"; echo "$3" >> "$root/deleted.log";;
"logs "*|"logs")
  # A test that wants a log of its own says so, and writes it.
  if [ -n "${OPENSHELL_FAKE_EVENTS:-}" ]; then cat "$root/${@: -1}/events.log" 2>/dev/null; exit 0; fi
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
