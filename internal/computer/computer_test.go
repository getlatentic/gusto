package computer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

func open(t *testing.T, org string) *Registry {
	t.Helper()
	r, err := Open(filepath.Join(t.TempDir(), "computers"), org)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func limited(t *testing.T, r *Registry, q quota.Quota) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, q); err != nil {
		t.Fatal(err)
	}
	r.UseQuota(quota.Open(path))
	return path
}

func TestASandboxNameFitsAGatewayAndSaysWhichOrganisationItIs(t *testing.T) {
	name := Name("acme", "ada@acme.com")
	if len(name) != 19 || !strings.HasPrefix(name, Marker) || !strings.HasPrefix(name, Prefix("acme")) {
		t.Errorf("%q", name)
	}
	if name != Name("acme", "ada@acme.com") {
		t.Error("the same person is the same name each time")
	}
	if name == Name("acme", "bob@acme.com") || Name("acme", "ada@acme.com") == Name("globex", "ada@acme.com") {
		t.Error("two people, and one person in two organisations, are different computers")
	}
	if Prefix("acme-research") == Prefix("acme-rocket") {
		t.Error("two organisations whose addresses begin alike do not share a prefix")
	}
	if strings.HasPrefix(name, "haas-") {
		t.Error("a computer's name is not a job's")
	}
}

func TestAPersonHasOneComputerAndAskingAgainFindsIt(t *testing.T) {
	r := open(t, "acme")
	first, made, err := r.Ensure("Ada@Acme.com", "cell-1")
	if err != nil || !made {
		t.Fatalf("%+v %v %v", first, made, err)
	}
	if first.User != "ada@acme.com" || first.Org != "acme" || first.State != Creating || first.Host != "cell-1" || first.Sandbox != Name("acme", "ada@acme.com") {
		t.Errorf("%+v", first)
	}
	again, made, err := r.Ensure(" ADA@acme.com ", "cell-2")
	if err != nil || made || again.ID != first.ID || again.Host != "cell-1" {
		t.Errorf("a second ask finds the first: %+v %v %v", again, made, err)
	}
	if _, _, err := r.Ensure("  ", "cell-1"); err == nil {
		t.Error("a computer is a person's")
	}
	if info, err := os.Stat(r.file(first.ID)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("%v %v", info, err)
	}
	if got, err := r.Of("ada@acme.com"); err != nil || got.ID != first.ID {
		t.Errorf("%+v %v", got, err)
	}
	if _, err := r.Of("nobody@acme.com"); !errors.Is(err, ErrNone) {
		t.Errorf("%v", err)
	}
}

func TestARecordIsOfOneOrganisationAndRefusesAnotherOnes(t *testing.T) {
	r := open(t, "acme")
	other := &Registry{dir: r.dir, org: "globex", now: time.Now}
	made, _, err := other.Ensure("bob@globex.com", "cell-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Change(made.ID, func(c *Computer) { c.State = Running }); err == nil || !strings.Contains(err.Error(), `"globex"`) {
		t.Errorf("a computer of another organisation cannot be changed through this record: %v", err)
	}
	if all, err := r.All(); err == nil {
		t.Errorf("a record with another organisation's computer in it is refused, not read past: %v %v", all, err)
	}
}

func TestTheNumberOfComputersIsUnderTheOrganisationsQuota(t *testing.T) {
	r := open(t, "acme")
	path := limited(t, r, quota.Quota{Computers: 2, Note: "Ask the operator for more."})
	a, _, _ := r.Ensure("a@acme.com", "cell-1")
	if _, _, err := r.Ensure("b@acme.com", "cell-1"); err != nil {
		t.Fatal(err)
	}
	_, _, err := r.Ensure("c@acme.com", "cell-1")
	if !errors.Is(err, quota.ErrExceeded) || !strings.Contains(err.Error(), "2 computers and has 2") || !strings.Contains(err.Error(), "Ask the operator") {
		t.Fatalf("the third is over the quota: %v", err)
	}
	if got, made, err := r.Ensure("a@acme.com", "cell-1"); err != nil || made || got.ID != a.ID {
		t.Errorf("a person who has one is not counted twice: %v %v", made, err)
	}
	// Removing one frees a place, and the person can have another later.
	if _, err := r.Change(a.ID, func(c *Computer) { c.State = Deleted }); err != nil {
		t.Fatal(err)
	}
	if all, _ := r.All(); len(all) != 1 {
		t.Errorf("a deleted computer is not listed: %d", len(all))
	}
	if _, _, err := r.Ensure("c@acme.com", "cell-1"); err != nil {
		t.Errorf("a place was freed: %v", err)
	}
	again, made, err := r.Ensure("a@acme.com", "cell-1")
	if !errors.Is(err, quota.ErrExceeded) {
		t.Fatalf("their old place is taken: %v %v %v", again, made, err)
	}

	// A quota that cannot be read makes no computer.
	os.Remove(path)
	time.Sleep(1100 * time.Millisecond)
	if _, _, err := r.Ensure("d@acme.com", "cell-1"); err == nil || errors.Is(err, quota.ErrExceeded) || !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("a quota that cannot be read is a fault: %v", err)
	}
}

func TestAPersonWhoseComputerWasDeletedGetsANewOne(t *testing.T) {
	r := open(t, "acme")
	first, _, _ := r.Ensure("a@acme.com", "cell-1")
	r.Change(first.ID, func(c *Computer) { c.State = Deleted })
	second, made, err := r.Ensure("a@acme.com", "cell-2")
	if err != nil || !made || second.State != Creating || second.Host != "cell-2" || second.ID != first.ID {
		t.Errorf("%+v %v %v", second, made, err)
	}
}

func TestTwoAddressesThatHashTheSameShareNoComputer(t *testing.T) {
	r := open(t, "acme")
	first, _, _ := r.Ensure("a@acme.com", "cell-1")
	if _, err := r.Change(first.ID, func(c *Computer) { c.User = "someone.else@acme.com" }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Ensure("a@acme.com", "cell-1"); err == nil || !strings.Contains(err.Error(), "no computer is shared") {
		t.Errorf("a record of one person is never handed to another: %v", err)
	}
}

func TestComputersAreListedOldestFirstAndOnlyThoseThatAreLive(t *testing.T) {
	r := open(t, "acme")
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	for _, user := range []string{"c@acme.com", "a@acme.com", "b@acme.com"} {
		r.Ensure(user, "cell-1")
		now = now.Add(time.Minute)
	}
	all, _ := r.All()
	if len(all) != 3 || all[0].User != "c@acme.com" || all[2].User != "b@acme.com" {
		t.Errorf("%+v", all)
	}
}

// Two requests for one person's computer at once make one; and with a quota,
// however many people ask at once, the quota is never passed.
func TestComputersAskedForTogetherAreNeverMoreThanAreAllowed(t *testing.T) {
	r := open(t, "acme")
	limited(t, r, quota.Quota{Computers: 5})
	var wg sync.WaitGroup
	var mu sync.Mutex
	made := 0
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, isNew, err := r.Ensure(fmt.Sprintf("u%d@acme.com", i%20), "cell-1")
			if err == nil && isNew {
				mu.Lock()
				made++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if all, _ := r.All(); made != 5 || len(all) != 5 {
		t.Errorf("%d were made and %d are listed, with a quota of 5", made, len(all))
	}
}

func TestTheRecordAndTheGatewayAreComparedForThisOrganisationOnly(t *testing.T) {
	r := open(t, "acme")
	running, _, _ := r.Ensure("run@acme.com", "cell-1")
	r.Change(running.ID, func(c *Computer) { c.State = Running })
	lost, _, _ := r.Ensure("lost@acme.com", "cell-1")
	r.Change(lost.ID, func(c *Computer) { c.State = Running })
	r.Ensure("new@acme.com", "cell-1") // still being made
	gone, _, _ := r.Ensure("gone@acme.com", "cell-1")
	r.Change(gone.ID, func(c *Computer) { c.State = Deleted })

	gateway := []string{
		running.Sandbox,
		Prefix("acme") + "ffffffff", // one of ours the record does not know
		Name("globex", "bob@globex.com"),
		"haas-1a2b3c4d5e6f", // a job's
	}
	drift, err := r.Compare(gateway)
	if err != nil {
		t.Fatal(err)
	}
	if len(drift.Missing) != 1 || drift.Missing[0].User != "lost@acme.com" {
		t.Errorf("only a computer that should be there and is not is missing: %+v", drift.Missing)
	}
	if len(drift.Orphans) != 1 || drift.Orphans[0] != Prefix("acme")+"ffffffff" {
		t.Errorf("only a sandbox that is this organisation's and unrecorded is an orphan: %v", drift.Orphans)
	}
	if drift.Clean() {
		t.Error("they disagree")
	}
	clean, _ := open(t, "acme").Compare([]string{"haas-1a2b3c4d5e6f"})
	if !clean.Clean() {
		t.Errorf("an empty record and a gateway with only jobs agree: %+v", clean)
	}
}
