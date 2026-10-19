package setup

import (
	"os"
	"testing"
	"time"
)

func shipped(body string) Setup {
	return Setup{Name: "health", Starters: []string{"Draft a note"}, Skills: []Skill{
		{Name: "phi-guard", Description: "Use on clinical text.", Body: body},
	}}
}

func bodyOf(t *testing.T, store *Store, skill string) string {
	t.Helper()
	held, _ := store.Get("health")
	for _, kept := range held.Skills {
		if kept.Name == skill {
			return kept.Body
		}
	}
	t.Fatalf("no skill %s", skill)
	return ""
}

// A skill nobody changed takes the version a newer build ships.
func TestAShippedSkillTakesTheNewerVersion(t *testing.T) {
	store, _ := Open(t.TempDir())
	if err := store.Provide(shipped("Replace names."), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Provide(shipped("Replace names, in replies too."), nil); err != nil {
		t.Fatal(err)
	}
	if got := bodyOf(t, store, "phi-guard"); got != "Replace names, in replies too." {
		t.Errorf("body = %q, want the newer version", got)
	}
}

// A skill a person edited keeps their edit when a newer build ships.
func TestAnEditedSkillKeepsTheEdit(t *testing.T) {
	store, _ := Open(t.TempDir())
	if err := store.Provide(shipped("Replace names."), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Write("health", Skill{Name: "phi-guard", Description: "Use on clinical text.", Body: "Our own rule."}); err != nil {
		t.Fatal(err)
	}
	if err := store.Provide(shipped("Replace names, in replies too."), nil); err != nil {
		t.Fatal(err)
	}
	if got := bodyOf(t, store, "phi-guard"); got != "Our own rule." {
		t.Errorf("body = %q, want the person's edit kept", got)
	}
}

// A setup saved before gusto kept fingerprints takes the newer version only
// where its files are as old as the setup, and a setup somebody else made is
// never touched.
func TestAnOlderSetupIsUpdatedOnlyWhereUntouched(t *testing.T) {
	store, _ := Open(t.TempDir())
	made := time.Now().Add(-time.Hour)
	if err := store.Save(Setup{Name: "health", Who: Provided, Made: made.Unix()}, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"phi-guard", "soap-note"} {
		if err := store.Write("health", Skill{Name: name, Description: "d", Body: "old"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(store.skillFile("health", "phi-guard"), made, made); err != nil {
		t.Fatal(err)
	}
	wanted := shipped("new")
	wanted.Skills = append(wanted.Skills, Skill{Name: "soap-note", Description: "d", Body: "new"})
	if err := store.Provide(wanted, nil); err != nil {
		t.Fatal(err)
	}
	if got := bodyOf(t, store, "phi-guard"); got != "new" {
		t.Errorf("an untouched skill = %q, want the newer version", got)
	}
	if got := bodyOf(t, store, "soap-note"); got != "old" {
		t.Errorf("a skill written after the setup was made = %q, want it kept", got)
	}

	if err := store.Save(Setup{Name: "mine", Who: "someone"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Provide(Setup{Name: "mine", Skills: []Skill{{Name: "x", Description: "d", Body: "b"}}}, nil); err != nil {
		t.Fatal(err)
	}
	if held, _ := store.Get("mine"); len(held.Skills) != 0 || held.Who != "someone" {
		t.Errorf("somebody else's setup was changed: %+v", held)
	}
}
