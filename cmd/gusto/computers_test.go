package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/pkg/job"
)

// hostProcesses is the process limit this machine already has. The fake gateway runs a sandbox's commands here, where the
// test's user already has many processes, so a sandbox is given this limit: the command that sets it still runs, and
// changes nothing.
func hostProcesses(t *testing.T) int {
	t.Helper()
	out, err := exec.Command("bash", "-c", "ulimit -u").Output()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// A gateway that keeps each sandbox as a directory, enough for a job to run in one.
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

// confinerShim stands in for gusto sandbox-run in the fake gateway, which runs commands on this machine: it writes down the policy
// and the home it was given, and runs the command unconfined.
func confinerShim(t *testing.T, root string) string {
	t.Helper()
	shim := filepath.Join(t.TempDir(), "confine")
	script := "#!/bin/sh\n[ \"$1\" = sandbox-run ] || exit 9\nprintf '%s\\n%s\\n' \"$2\" \"$HOME\" > " + root + "/confined-$$\nshift 3\nexec \"$@\"\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return shim
}

func withGateway(t *testing.T) string {
	t.Helper()
	bin, root := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "openshell"), []byte(gateway), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSHELL_FAKE_ROOT", root)
	return root
}

// organizationServer is what serve is made of for an organization that runs jobs in
// computers: the index, the way out, the record of computers, and the work that runs a job.
func organizationServer(t *testing.T, quotaOf quota.Quota) (jobs.Work, *jobs.Index, *computer.Registry, openshell.Config, string) {
	work, index, registry, cfg, root, _ := organizationWithInstances(t, quotaOf)
	return work, index, registry, cfg, root
}

func organizationWithInstances(t *testing.T, quotaOf quota.Quota) (jobs.Work, *jobs.Index, *computer.Registry, openshell.Config, string, *instance.Store) {
	t.Helper()
	root := withGateway(t)
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "fixture.json"), []byte(`{"name":"fixture","program":"sh","argv":["-c","printf yes > answer.txt; echo ran in $(pwd | sed 's|.*/sandbox||')"],"needs":"nothing","stream":"plain"}`), 0o644)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	t.Setenv("GUSTO_SANDBOX", "openshell")
	t.Setenv("GUSTO_MODEL_UPSTREAM", "http://127.0.0.1:9")
	t.Setenv("GUSTO_MODEL_KEY", "not-a-key")
	t.Setenv("GUSTO_PROXY_ADDR", "127.0.0.1:0")
	t.Setenv("GUSTO_CREDENTIALS", "")
	index, err := jobs.Open(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	way, err := openWayOut(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	limit := filepath.Join(t.TempDir(), "quota.json")
	quota.Write(limit, quotaOf)
	registry.UseQuota(quota.Open(limit))
	cfg := openshell.Config{Binary: "openshell", Image: "runner.tar", Host: "host.openshell.internal", CPU: "1", Memory: "1Gi", Processes: hostProcesses(t),
		Confiner: confinerShim(t, root)}
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	work := inSandbox(way, cfg, index, registry, instances, newRoomRuns(instances, func(dir job.Dir) string { return personWhoAsked(index, dir) }))
	return func(dir job.Dir) (int, error) { return work(context.Background(), dir) }, index, registry, cfg, root, instances
}

// jobBy is a job asked for by who, run by the work, and the directory it ran in.
func jobBy(t *testing.T, work jobs.Work, index *jobs.Index, who, brief string) job.Dir {
	t.Helper()
	id, err := index.Reserve(brief, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	record := jobs.Record{ID: id, State: jobs.Running, Brief: brief, Harness: "fixture", Asked: who, Queued: jobs.Now()}
	dir := record.Dir(index.Root())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (job.Spec{Brief: brief, Harness: "fixture", Gate: []string{"true"}}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	if _, err := work(dir); err != nil {
		t.Logf("%s: %v", who, err)
	}
	return dir
}

func transcriptOf(dir job.Dir) string {
	text, _ := os.ReadFile(dir.OutFile("transcript.log"))
	return string(text)
}

func TestAPersonsJobsRunInTheirOwnComputerAndAScriptsJobInASandboxOfItsOwn(t *testing.T) {
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 4, Jobs: 100})
	adaOne := jobBy(t, work, index, "ada@acme.com", "first thing for ada")
	adaTwo := jobBy(t, work, index, "Ada@Acme.com", "second thing for ada")
	bob := jobBy(t, work, index, "bob@acme.com", "a thing for bob")
	script := jobBy(t, work, index, "token:ci", "a thing for a script")

	ada, err := registry.Of("ada@acme.com")
	if err != nil || ada.State != computer.Running || ada.Org != "acme-com" || ada.Sandbox != computer.Name("acme-com", "ada@acme.com") || ada.LastActive.IsZero() {
		t.Fatalf("Ada has a computer that ran her jobs: %+v %v", ada, err)
	}
	bobs, _ := registry.Of("bob@acme.com")
	if bobs.ID == ada.ID || bobs.Sandbox == ada.Sandbox {
		t.Error("two people share a computer")
	}
	if all, _ := registry.All(); len(all) != 2 {
		t.Errorf("a script has no computer, and the address is one person's in any case: %d computers", len(all))
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if n := strings.Count(string(calls), "sandbox create --name "+ada.Sandbox); n != 1 {
		t.Errorf("Ada's computer was made %d times", n)
	}
	for name, want := range map[string]string{adaOne.Path(): "ran in /jobs/", adaTwo.Path(): "ran in /jobs/", bob.Path(): "ran in /jobs/"} {
		if text := transcriptOf(job.Dir(name)); !strings.Contains(text, want) {
			t.Errorf("%s: %q", filepath.Base(name), text)
		}
	}
	if text := transcriptOf(script); !strings.Contains(text, "ran in /workspace") {
		t.Errorf("a script's job runs where jobs always ran, in a sandbox of its own: %q", text)
	}
	deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log"))
	if strings.Contains(string(deleted), ada.Sandbox) || strings.Contains(string(deleted), bobs.Sandbox) || len(strings.Fields(string(deleted))) != 1 {
		t.Errorf("only the script's sandbox went when its job did: %q", deleted)
	}
}

func TestAPersonPastTheOrganizationsComputersIsRefusedInTheRecordOfTheirTask(t *testing.T) {
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 1, Jobs: 100, Note: "Ask the admin for more."})
	jobBy(t, work, index, "ada@acme.com", "the first")
	carol := jobBy(t, work, index, "carol@acme.com", "the second")
	text := transcriptOf(carol)
	if !strings.Contains(text, "refused this task before the agent started") || !strings.Contains(text, "may have 1 workspace and has 1") || !strings.Contains(text, "Ask the admin for more.") {
		t.Errorf("%q", text)
	}
	if _, err := registry.Of("carol@acme.com"); err == nil {
		t.Error("a refused person was given a computer")
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if n := strings.Count(string(calls), "sandbox create"); n != 1 {
		t.Errorf("nothing was made for the refused job: %d", n)
	}
}

func TestAComputerThatCannotBeMadeIsFailedInTheRecordWithTheReason(t *testing.T) {
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 4, Jobs: 100})
	os.WriteFile(filepath.Join(root, "unused"), nil, 0o644)
	// The gateway can make nothing.
	bad := filepath.Join(t.TempDir(), "openshell")
	os.WriteFile(bad, []byte("#!/bin/bash\necho \"Error: × the gateway is down\" >&2\nexit 1\n"), 0o755)
	t.Setenv("PATH", filepath.Dir(bad)+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := jobBy(t, work, index, "ada@acme.com", "a thing")
	got, err := registry.Of("ada@acme.com")
	if err != nil || got.State != computer.Failed || !strings.Contains(got.Note, "gateway is down") {
		t.Errorf("%+v %v", got, err)
	}
	_ = dir
}

func TestTheJanitorRemovesRetiredComputersAndStopsIdleOnesThatNoOneIsUsing(t *testing.T) {
	work, index, registry, cfg, root := organizationServer(t, quota.Quota{Computers: 4, Jobs: 100})
	jobBy(t, work, index, "ada@acme.com", "for ada")
	jobBy(t, work, index, "bob@acme.com", "for bob")
	jobBy(t, work, index, "cy@acme.com", "for cy")
	jobBy(t, work, index, "dee@acme.com", "for dee")
	ada, _ := registry.Of("ada@acme.com")
	bob, _ := registry.Of("bob@acme.com")
	cy, _ := registry.Of("cy@acme.com")
	dee, _ := registry.Of("dee@acme.com")

	// Ada was removed from the organization, by the front door, which only marks her computer.
	if err := registry.Retire("ada@acme.com"); err != nil {
		t.Fatal(err)
	}
	// Bob has been idle and cy has too, but cy has a job running.
	old := time.Now().Add(-2 * time.Hour)
	for _, c := range []computer.Computer{bob, cy} {
		registry.Change(c.ID, func(each *computer.Computer) { each.LastActive = old })
	}
	retire(cfg, registry)
	rest(cfg, registry, func(person string) bool { return person == "cy@acme.com" }, time.Hour)

	deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log"))
	if !strings.Contains(string(deleted), ada.Sandbox) {
		t.Errorf("a retired computer is removed from the gateway: %q", deleted)
	}
	if _, err := registry.Of("ada@acme.com"); err == nil {
		t.Error("and from the record")
	}
	if _, err := os.Stat(filepath.Join(root, bob.Sandbox, "stopped")); err != nil {
		t.Error("an idle computer nobody is using is stopped")
	}
	if got, _ := registry.Of("bob@acme.com"); got.State != computer.Stopped {
		t.Errorf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, cy.Sandbox, "stopped")); err == nil {
		t.Error("a computer with a job running is left running")
	}
	if got, _ := registry.Of("cy@acme.com"); got.State != computer.Running {
		t.Errorf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, dee.Sandbox, "stopped")); err == nil {
		t.Error("a computer that was used a moment ago is not idle")
	}
	// And a stopped computer is started for the next job.
	jobBy(t, work, index, "bob@acme.com", "bob again")
	if _, err := os.Stat(filepath.Join(root, bob.Sandbox, "stopped")); err == nil {
		t.Error("Bob's computer was not started for his job")
	}
	if got, _ := registry.Of("bob@acme.com"); got.State != computer.Running {
		t.Errorf("%+v", got)
	}
}
