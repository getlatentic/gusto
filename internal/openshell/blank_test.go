package openshell_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/spare"
	"github.com/getlatentic/gusto/pkg/harness"
)

const (
	model = "http://host.openshell.internal:8041/model/v1"
	org   = "acme"
)

var blankConfig = openshell.Config{Binary: "openshell", Image: "runner.tar", CPU: "1", Memory: "1Gi"}

func theSandboxes(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "sandbox")); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names
}

func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// What the gateway cannot change after a sandbox is made, the filesystem rules, has to be the same for a spare and for a
// person's computer, whichever of the built-in agents the person's task uses.
func TestAComputerMadeAheadHasThePolicyOfAPersonsForEveryAgentWhoseFoldersAreInTheHome(t *testing.T) {
	blank, err := openshell.ComputerPolicy(blankConfig, model, harness.Adapter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, keeps := range []string{"/root/.local/share/claude", "/root/.local/share/codex", ""} {
		theirs, err := openshell.ComputerPolicy(blankConfig, model, harness.Adapter{Keeps: keeps})
		if err != nil {
			t.Fatal(err)
		}
		if string(theirs) != string(blank) {
			t.Errorf("the policy for an agent that keeps %q differs from the policy of a computer made ahead:\n%s\n%s", keeps, theirs, blank)
		}
	}
	elsewhere, _ := openshell.ComputerPolicy(blankConfig, model, harness.Adapter{Keeps: "/opt/agent/state"})
	if openshell.PolicyKey(elsewhere) == openshell.PolicyKey(blank) {
		t.Error("an agent that writes outside the home shares a key with a computer that cannot let it")
	}
	other, _ := openshell.ComputerPolicy(blankConfig, "http://host.openshell.internal:8042/model/v1", harness.Adapter{})
	if openshell.PolicyKey(other) == openshell.PolicyKey(blank) {
		t.Error("a computer made for one organization's proxy shares a key with another's")
	}
}

func TestABlankComputerHoldsNothingOfAPersonAndIsMadeFromTheConfiguredImage(t *testing.T) {
	root := installFake(t)
	if err := openshell.MakeBlank(context.Background(), blankConfig, "s-1234abcd-0000aaaa", model); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	for _, want := range []string{"sandbox create --name s-1234abcd-0000aaaa", "--cpu 1 --memory 1Gi", "--from runner.tar", "--no-auto-providers"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("the gateway was not asked for %q:\n%s", want, calls)
		}
	}
	left, _ := os.ReadDir(filepath.Join(root, "s-1234abcd-0000aaaa", "sandbox"))
	for _, entry := range left {
		if entry.Name() != "workspace" {
			t.Errorf("%s is in a computer nobody has used", entry.Name())
		}
	}
	policy, _ := os.ReadFile(filepath.Join(root, "s-1234abcd-0000aaaa", "policy.json"))
	want, _ := openshell.ComputerPolicy(blankConfig, model, harness.Adapter{})
	if string(policy) != string(want) {
		t.Errorf("the policy the gateway was given is not a computer's:\n%s", policy)
	}
}

func TestNamedListsOnlyTheSandboxesWithThePrefixAndRemoveIgnoresOneThatIsGone(t *testing.T) {
	installFake(t)
	for _, name := range []string{"s-1234abcd-0000aaaa", "s-1234abcd-0000bbbb", "c-1234abcd-ffffffff", "s-99999999-0000aaaa"} {
		if err := openshell.MakeBlank(context.Background(), blankConfig, name, model); err != nil {
			t.Fatal(err)
		}
	}
	found, err := openshell.Named(context.Background(), blankConfig, "s-1234abcd-")
	if err != nil || len(found) != 2 {
		t.Fatalf("%v %v", found, err)
	}
	if err := openshell.Remove(context.Background(), blankConfig, "s-1234abcd-0000aaaa"); err != nil {
		t.Error(err)
	}
	if err := openshell.Remove(context.Background(), blankConfig, "s-1234abcd-0000aaaa"); err != nil {
		t.Errorf("removing what is gone: %v", err)
	}
}

// spares is a pool on the fake gateway, as the server makes one.
func spares(t *testing.T) *spare.Pool {
	t.Helper()
	key, err := openshell.BlankKey(blankConfig, model)
	if err != nil {
		t.Fatal(err)
	}
	maker := spare.Maker{
		Name: func() string { return computer.SpareName(org) },
		Make: func(ctx context.Context, name string) error {
			return openshell.MakeBlank(ctx, blankConfig, name, model)
		},
		Remove: func(ctx context.Context, name string) error { return openshell.Remove(ctx, blankConfig, name) },
		Stale: func(ctx context.Context) ([]string, error) {
			return openshell.Named(ctx, blankConfig, computer.SparePrefix(org))
		},
	}
	pool := spare.Open(context.Background(), key, maker, spare.Timing{MakeWithin: 30 * time.Second, RemoveWithin: 30 * time.Second, RetryFirst: 10 * time.Millisecond, RetryMost: 50 * time.Millisecond})
	t.Cleanup(func() { pool.Close(10 * time.Second) })
	return pool
}

func TestAFirstTaskRunsInTheSpareWithoutMakingASandboxAndAnotherSpareIsMade(t *testing.T) {
	root := installFake(t)
	pool := spares(t)
	key, _ := openshell.BlankKey(blankConfig, model)
	eventually(t, "the first spare", func() bool { return len(theSandboxes(t, root)) == 1 })
	spareName := theSandboxes(t, root)[0]

	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), org)
	if err != nil {
		t.Fatal(err)
	}
	// The spare is on the pool's shelf a moment after the gateway has it, and a person asking before is made a computer of their own.
	var held computer.Computer
	for i := 0; held.Sandbox != spareName; i++ {
		if i > 2000 {
			t.Fatalf("nobody was given the spare %s", spareName)
		}
		if held, _, err = registry.EnsureFrom(fmt.Sprintf("p%d@acme.com", i), "cell-1", spare.Offer{Pool: pool, Key: key}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	dir, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt; ls -A ../../../.. | tr '\n' ' ' > seen.txt`)
	if exit, err := runner.Outside(dir, inComputer(t, dir, held.Sandbox, "20260929-1200-first")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	for _, line := range strings.Split(string(calls), "\n") {
		if strings.HasPrefix(line, "sandbox create --name "+computer.Name(org, held.User)) {
			t.Errorf("the task made a sandbox of its own: %s", line)
		}
	}
	if _, err := os.Stat(filepath.Join(root, computer.Name(org, held.User))); err == nil {
		t.Error("a sandbox was made under the person's own name as well as the spare")
	}
	eventually(t, "the next spare", func() bool {
		names := theSandboxes(t, root)
		return len(names) == 2
	})
	for _, name := range theSandboxes(t, root) {
		if name != spareName && !strings.HasPrefix(name, computer.SparePrefix(org)) {
			t.Errorf("%s is neither the person's computer nor a spare", name)
		}
	}
}

func TestStoppingThePoolLeavesNoSpareOnTheGateway(t *testing.T) {
	root := installFake(t)
	pool := spares(t)
	eventually(t, "the first spare", func() bool { return len(theSandboxes(t, root)) == 1 })
	pool.Close(10 * time.Second)
	if left := theSandboxes(t, root); len(left) != 0 {
		t.Errorf("%v left on the gateway", left)
	}
}

func TestASpareThatCannotBeMadeLeavesNothingAndTheFirstTaskMakesItsOwn(t *testing.T) {
	root := installFake(t)
	if err := os.WriteFile(filepath.Join(root, "broken"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	pool := spares(t)
	key, _ := openshell.BlankKey(blankConfig, model)
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), org)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	held, _, err := registry.EnsureFrom("ada@acme.com", "cell-1", spare.Offer{Pool: pool, Key: key})
	if err != nil || held.Sandbox != computer.Name(org, "ada@acme.com") {
		t.Fatalf("%q %v", held.Sandbox, err)
	}
	os.Remove(filepath.Join(root, "broken"))
	dir, _ := aJob(t)
	withHarness(t, `printf yes > answer.txt`)
	if exit, err := runner.Outside(dir, inComputer(t, dir, held.Sandbox, "20260929-1200-first")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	if _, err := os.Stat(filepath.Join(root, held.Sandbox, "sandbox")); err != nil {
		t.Error("the person's own computer was not made")
	}
}
