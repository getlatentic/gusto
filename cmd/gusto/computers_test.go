package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/backend/vm"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/openshell/openshelltest"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
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

// gustoShim stands in for gusto in the fake gateway's sandbox, which runs commands on this machine: it writes down the Landlock
// policy and the home it was given, and runs the real gusto boxed, the test binary, without the policy, which Linux alone applies.
func gustoShim(t *testing.T, root string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(t.TempDir(), "gusto")
	script := `#!/bin/sh
if [ "$1" = sandbox-run ]; then printf '%s\n%s\n' "$2" "$HOME" > ` + root + `/confined-$$; shift 3; exec "$@"; fi
[ "$1" = boxed ] || exit 9; shift
model=""; policy=""
while [ "$1" != "--" ]; do case "$1" in --model) model=$2; shift 2;; --policy) policy=$2; shift 2;; *) exit 9;; esac; done
shift
[ -z "$policy" ] || printf '%s\n%s\n' "$policy" "$HOME" > ` + root + `/confined-$$
exec ` + self + ` boxed --model "$model" -- "$@"
`
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return shim
}

// freeAddress is an address on this machine's loopback that nothing listens on.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

// organizationServer is what serve is made of for an organization that runs tasks in
// computers: the index, the way out, the record of computers, and the work that runs a task.
func organizationServer(t *testing.T, quotaOf quota.Quota) (tasks.Work, *tasks.Index, *computer.Registry, openshell.Config, string) {
	work, index, registry, cfg, root, _ := organizationWithInstances(t, quotaOf)
	return work, index, registry, cfg, root
}

func organizationWithInstances(t *testing.T, quotaOf quota.Quota) (tasks.Work, *tasks.Index, *computer.Registry, openshell.Config, string, *instance.Store) {
	t.Helper()
	return organizationWithWayOut(t, quotaOf, func(*WayOut) {})
}

// organizationWithWayOut is organizationWithInstances with the server's way out set up by configure before any task runs.
func organizationWithWayOut(t *testing.T, quotaOf quota.Quota, configure func(*WayOut)) (tasks.Work, *tasks.Index, *computer.Registry, openshell.Config, string, *instance.Store) {
	t.Helper()
	root := openshelltest.Install(t)
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "fixture.json"), []byte(`{"name":"fixture","program":"sh","argv":["-c","printf yes > answer.txt; echo ran in $(pwd | sed 's|.*/sandbox||')"],"needs":"nothing","stream":"plain"}`), 0o644)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	t.Setenv("GUSTO_SANDBOX", "openshell")
	t.Setenv("GUSTO_MODEL_UPSTREAM", "http://127.0.0.1:9")
	t.Setenv("GUSTO_MODEL_KEY", "not-a-key")
	t.Setenv("GUSTO_PROXY_ADDR", freeAddress(t))
	t.Setenv("GUSTO_CREDENTIALS", "")
	index, err := tasks.Open(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	way, err := openWayOut(index.Root(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	configure(way)
	// An agent in a sandbox reaches its model through its forwarder, which trades its secret at the proxy before it starts.
	stopProxy, err := way.Listen(backend.Loopback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopProxy)
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	limit := filepath.Join(t.TempDir(), "quota.json")
	quota.Write(limit, quotaOf)
	registry.UseQuota(quota.Open(limit))
	cfg := openshell.Config{Binary: "openshell", Image: "runner.tar", Host: "127.0.0.1", CPU: "1", Memory: "1Gi", Processes: hostProcesses(t),
		Gusto: gustoShim(t, root)}
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	way.UseRunRecords(func(dir task.Dir, spec task.Spec) evidence.Run { return runRecord(index, instances, dir, spec) })
	boxes := vm.New(vm.Config{Gateway: cfg, Computers: registry, Org: "acme-com", Node: "local"})
	work := turns(boxes, way, instances, index, newRoomRuns(instances, func(dir task.Dir) string { return personWhoAsked(index, dir) }, fromEnv(t).boxes.roomCopy(), nil))
	return func(dir task.Dir) (int, error) { return work(context.Background(), dir) }, index, registry, cfg, root, instances
}

// taskBy is a task asked for by who, run by the work, and the directory it ran in.
func taskBy(t *testing.T, work tasks.Work, index *tasks.Index, who, brief string) task.Dir {
	t.Helper()
	id, err := index.Reserve(brief, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	record := tasks.Record{ID: id, State: tasks.Running, Brief: brief, Harness: "fixture", Asked: who, Queued: tasks.Now()}
	dir := record.Dir(index.Root())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (task.Spec{Brief: brief, Harness: "fixture", Gate: []string{"true"}}).Write(dir); err != nil {
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

func transcriptOf(dir task.Dir) string {
	text, _ := os.ReadFile(dir.OutFile("transcript.log"))
	return string(text)
}

func TestAPersonsTasksRunInTheirOwnComputerAndAScriptsTaskInASandboxOfItsOwn(t *testing.T) {
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 4, Tasks: 100})
	adaOne := taskBy(t, work, index, "ada@acme.com", "first thing for ada")
	adaTwo := taskBy(t, work, index, "Ada@Acme.com", "second thing for ada")
	bob := taskBy(t, work, index, "bob@acme.com", "a thing for bob")
	script := taskBy(t, work, index, "token:ci", "a thing for a script")

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
		if text := transcriptOf(task.Dir(name)); !strings.Contains(text, want) {
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
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 1, Tasks: 100, Note: "Ask the admin for more."})
	taskBy(t, work, index, "ada@acme.com", "the first")
	carol := taskBy(t, work, index, "carol@acme.com", "the second")
	text := transcriptOf(carol)
	if !strings.Contains(text, "refused this task before the agent started") || !strings.Contains(text, "may have 1 computer and has 1") || !strings.Contains(text, "Ask the admin for more.") {
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
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 4, Tasks: 100})
	os.WriteFile(filepath.Join(root, "unused"), nil, 0o644)
	// The gateway can make nothing.
	bad := filepath.Join(t.TempDir(), "openshell")
	os.WriteFile(bad, []byte("#!/bin/bash\necho \"Error: × the gateway is down\" >&2\nexit 1\n"), 0o755)
	t.Setenv("PATH", filepath.Dir(bad)+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := taskBy(t, work, index, "ada@acme.com", "a thing")
	got, err := registry.Of("ada@acme.com")
	if err != nil || got.State != computer.Failed || !strings.Contains(got.Note, "gateway is down") {
		t.Errorf("%+v %v", got, err)
	}
	_ = dir
}

// A computer stopped for being idle is started again for its person's next task.
func TestAStoppedComputerIsStartedForThePersonsNextTask(t *testing.T) {
	work, index, registry, _, root := organizationServer(t, quota.Quota{Computers: 4, Tasks: 100})
	taskBy(t, work, index, "bob@acme.com", "for bob")
	bob, _ := registry.Of("bob@acme.com")
	if err := os.WriteFile(filepath.Join(root, bob.Sandbox, "stopped"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	registry.Change(bob.ID, func(each *computer.Computer) { each.State = computer.Stopped })
	taskBy(t, work, index, "bob@acme.com", "bob again")
	if _, err := os.Stat(filepath.Join(root, bob.Sandbox, "stopped")); err == nil {
		t.Error("Bob's computer was not started for his job")
	}
	if got, _ := registry.Of("bob@acme.com"); got.State != computer.Running {
		t.Errorf("%+v", got)
	}
}
