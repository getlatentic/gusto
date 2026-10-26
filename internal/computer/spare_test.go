package computer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/quota"
)

// shelf is spares that are handed out once each, and given back.
type shelf struct {
	mu       sync.Mutex
	names    []string
	returned []string
}

func (s *shelf) Take() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.names) == 0 {
		return "", false
	}
	name := s.names[0]
	s.names = s.names[1:]
	return name, true
}

func (s *shelf) Return(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.returned = append(s.returned, name)
}

func TestASpareSandboxNameFitsAGatewayAndIsNotAComputers(t *testing.T) {
	name := SpareName("acme")
	if len(name) != 19 || !strings.HasPrefix(name, SparePrefix("acme")) {
		t.Errorf("%q", name)
	}
	if strings.HasPrefix(name, Prefix("acme")) || strings.HasPrefix(name, "gusto-") {
		t.Errorf("%q would be taken for a computer or a task", name)
	}
	if name == SpareName("acme") {
		t.Error("two spares have the same name")
	}
	if SparePrefix("acme") == SparePrefix("globex") {
		t.Error("two organizations share a spare prefix")
	}
}

func TestANewComputerIsGivenTheSpareAndRecordsItsName(t *testing.T) {
	r := open(t, "acme")
	spares := &shelf{names: []string{"s-spare-one"}}
	made, isNew, err := r.EnsureFrom("ada@acme.com", "cell-1", spares)
	if err != nil || !isNew {
		t.Fatalf("%v %v", isNew, err)
	}
	if made.Sandbox != "s-spare-one" {
		t.Errorf("the computer is %q, want the spare", made.Sandbox)
	}
	found, err := r.Of("ada@acme.com")
	if err != nil || found.Sandbox != "s-spare-one" {
		t.Errorf("the record says %q, %v", found.Sandbox, err)
	}
}

func TestACompanyWithNoSpareMakesTheComputerUnderItsOwnName(t *testing.T) {
	r := open(t, "acme")
	made, _, err := r.EnsureFrom("ada@acme.com", "cell-1", &shelf{})
	if err != nil || made.Sandbox != Name("acme", "ada@acme.com") {
		t.Errorf("%q %v", made.Sandbox, err)
	}
	plain, _, err := r.EnsureFrom("bob@acme.com", "cell-1", nil)
	if err != nil || plain.Sandbox != Name("acme", "bob@acme.com") {
		t.Errorf("%q %v", plain.Sandbox, err)
	}
}

func TestAPersonWhoHasAComputerIsNotGivenASpare(t *testing.T) {
	r := open(t, "acme")
	if _, _, err := r.EnsureFrom("ada@acme.com", "cell-1", &shelf{names: []string{"s-first"}}); err != nil {
		t.Fatal(err)
	}
	again := &shelf{names: []string{"s-second"}}
	found, isNew, err := r.EnsureFrom("ada@acme.com", "cell-1", again)
	if err != nil || isNew || found.Sandbox != "s-first" {
		t.Errorf("%q new=%v %v", found.Sandbox, isNew, err)
	}
	if len(again.names) != 1 {
		t.Error("a person who already has a computer used up a spare")
	}
}

func TestAComputerThatWasRemovedIsGivenASpareToo(t *testing.T) {
	r := open(t, "acme")
	made, _, _ := r.EnsureFrom("ada@acme.com", "cell-1", nil)
	if _, err := r.Change(made.ID, func(c *Computer) { c.State = Deleted }); err != nil {
		t.Fatal(err)
	}
	again, isNew, err := r.EnsureFrom("ada@acme.com", "cell-1", &shelf{names: []string{"s-new"}})
	if err != nil || !isNew || again.Sandbox != "s-new" {
		t.Errorf("%q new=%v %v", again.Sandbox, isNew, err)
	}
}

func TestASpareIsGivenBackWhenTheComputerIsRefused(t *testing.T) {
	r := open(t, "acme")
	spares := &shelf{names: []string{"s-spare"}}
	if err := os.Chmod(r.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(r.dir, 0o700)
	if _, _, err := r.EnsureFrom("ada@acme.com", "cell-1", spares); err == nil {
		t.Skip("this user can write to a directory that has no write permission")
	}
	if len(spares.returned) != 1 || spares.returned[0] != "s-spare" {
		t.Errorf("returned %v: the spare was lost with the record that was never written", spares.returned)
	}
}

func TestAComputerPastTheLimitIsRefusedWithoutTakingASpare(t *testing.T) {
	r := open(t, "acme")
	limited(t, r, quota.Quota{Computers: 1})
	if _, _, err := r.Ensure("first@acme.com", "cell-1"); err != nil {
		t.Fatal(err)
	}
	spares := &shelf{names: []string{"s-spare"}}
	if _, _, err := r.EnsureFrom("ada@acme.com", "cell-1", spares); err == nil {
		t.Fatal("a computer past the limit was made")
	}
	if len(spares.names) != 1 {
		t.Error("a refused computer took the spare")
	}
}

func TestTwoPeopleAtOnceAreNotGivenTheSameSpare(t *testing.T) {
	r := open(t, "acme")
	spares := &shelf{names: []string{"s-only"}}
	var group sync.WaitGroup
	made := make([]Computer, 8)
	for i := range made {
		group.Add(1)
		go func() {
			defer group.Done()
			c, _, err := r.EnsureFrom(fmt.Sprintf("p%d@acme.com", i), "cell-1", spares)
			if err != nil {
				t.Error(err)
			}
			made[i] = c
		}()
	}
	group.Wait()
	got := 0
	for _, c := range made {
		if c.Sandbox == "s-only" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("%d people were given the one spare", got)
	}
	files, _ := filepath.Glob(filepath.Join(r.dir, "*.json"))
	if len(files) != len(made) {
		t.Errorf("%d records for %d people", len(files), len(made))
	}
}
