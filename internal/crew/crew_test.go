package crew

import (
	"strings"
	"testing"
)

func TestAnAgentIsMadeOnceAndChangedInPlace(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := store.Make(Agent{Name: " Tosin's Claude ", Harness: "claude", Owner: "Ada@Example.com", Access: []string{"bob@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if made.Name != "Tosin's Claude" || made.Owner != "ada@example.com" || len(made.Access) != 0 || !validID(made.ID) {
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
