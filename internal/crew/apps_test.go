package crew

import (
	"slices"
	"testing"
)

func opened(t *testing.T) (*Store, Agent) {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := store.Make(Agent{Name: "Nova", Harness: "codex", Owner: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return store, made
}

func access(t *testing.T, store *Store, id, who string, allowed bool) {
	t.Helper()
	if _, err := store.Change(id, func(held *Agent) error {
		held.Access = slices.DeleteFunc(held.Access, func(each string) bool { return each == who })
		if allowed {
			held.Access = append(held.Access, who)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAPersonTakenOffAnAgentLosesWhatTheyChoseForItsApps(t *testing.T) {
	store, made := opened(t)
	access(t, store, made.ID, "bob@example.com", true)
	if _, err := store.ChooseApp(made.ID, "bob@example.com", "notion", "allow"); err != nil {
		t.Fatal(err)
	}
	access(t, store, made.ID, "bob@example.com", false)
	access(t, store, made.ID, "bob@example.com", true)
	held, _ := store.Get(made.ID)
	if _, chosen := held.AppChoice("bob@example.com", "notion"); chosen {
		t.Error("a person let work with the agent again found what they chose before they were taken off")
	}
}

func TestAPersonWhoDoesNotWorkWithAnAgentDoesNotChooseForIt(t *testing.T) {
	store, made := opened(t)
	if _, err := store.ChooseApp(made.ID, "mallory@example.com", "notion", "allow"); err == nil {
		t.Error("a person who does not work with the agent chose for it")
	}
	held, _ := store.Get(made.ID)
	if _, chosen := held.AppChoice("mallory@example.com", "notion"); chosen {
		t.Error("their choice was kept")
	}
	if _, err := store.ChooseApp(made.ID, "ada@example.com", "notion", "allow"); err != nil {
		t.Errorf("the owner, who works with it, did not choose: %v", err)
	}
}
