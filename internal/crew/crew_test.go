package crew

import (
	"strings"
	"testing"
)

func TestAnAgentIsMadeOnceAndChangedInPlace(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	made, err := store.Make(Agent{Name: " Tosin's Claude ", Harness: "claude", Owner: "Ada@Example.com", Access: []string{"bob@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if made.Name != "Tosin's Claude" || made.Owner != "ada@example.com" || len(made.Access) != 0 || len(made.ID) != 12 {
		t.Fatalf("made %+v", made)
	}
	if _, err := store.Make(Agent{Name: "tosin's claude", Harness: "codex", Owner: "bob@example.com"}); err == nil || !strings.Contains(err.Error(), "already called") {
		t.Errorf("a second agent took the same name: %v", err)
	}
	changed, err := store.Change(made.ID, func(held *Agent) error {
		held.Model, held.Owner = "opus", "mallory@example.com"
		held.Access = []string{"Bob@example.com", "bob@example.com", "ada@example.com"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Owner != "ada@example.com" || changed.Model != "opus" || len(changed.Access) != 1 || changed.Access[0] != "bob@example.com" {
		t.Errorf("changed to %+v", changed)
	}
	if !changed.MayUse("ADA@example.com") || !changed.MayUse("bob@example.com") || changed.MayUse("carol@example.com") {
		t.Error("who may use the agent is wrong")
	}
	if got, _ := store.Get(made.ID); got.Model != "opus" || len(store.List()) != 1 {
		t.Errorf("the store holds %+v", store.List())
	}
	got, _ := store.Get(made.ID)
	got.Access[0] = "mallory@example.com"
	if again, _ := store.Get(made.ID); again.Access[0] != "bob@example.com" {
		t.Error("changing an agent a caller was given changed the one kept")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back, found := reopened.Get(made.ID); !found || back.Model != "opus" || back.Access[0] != "bob@example.com" {
		t.Errorf("read back from its folder, the agent is %+v", back)
	}
	if err := store.Remove(made.ID); err != nil || len(store.List()) != 0 {
		t.Errorf("removing left %+v: %v", store.List(), err)
	}
}

func TestAnAgentNeedsANamePeopleCanMention(t *testing.T) {
	store, _ := Open(t.TempDir())
	for _, name := range []string{"", "9lives", "@Scout", strings.Repeat("a", 33)} {
		if _, err := store.Make(Agent{Name: name, Harness: "claude", Owner: "ada@example.com"}); err == nil {
			t.Errorf("%q named an agent", name)
		}
	}
	if _, ok := store.Get("../etc/passwd"); ok {
		t.Error("a path was taken for an id")
	}
}
