package preset

import (
	"strings"
	"testing"
)

func TestAPresetIsKeptForItsPersonAndForNobodyElse(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Save("ada@acme.com", Preset{Name: "Reviewer", Harness: "claude", Model: "m", Role: "engineering"})
	if err != nil || saved.ID == "" {
		t.Fatalf("saving: %+v %v", saved, err)
	}
	if got := store.List("ada@acme.com"); len(got) != 1 || got[0].Name != "Reviewer" || got[0].Role != "engineering" {
		t.Errorf("ada's presets: %+v", got)
	}
	if got := store.List("bob@acme.com"); len(got) != 0 {
		t.Errorf("bob sees ada's presets: %+v", got)
	}
	if _, found := store.Get("bob@acme.com", saved.ID); found {
		t.Error("bob can read ada's preset by its id")
	}
	if got := store.List("ADA@acme.com"); len(got) != 1 {
		t.Error("an address is the same person whatever its case")
	}
}

func TestAPresetNeedsANameAnEngineAndAnUnusedName(t *testing.T) {
	store, _ := Open(t.TempDir())
	for name, wanted := range map[string]Preset{
		"no name":   {Harness: "claude"},
		"no engine": {Name: "x"},
		"too long":  {Name: strings.Repeat("a", MaxName+1), Harness: "claude"},
	} {
		if _, err := store.Save("ada", wanted); err == nil {
			t.Errorf("%s was saved", name)
		}
	}
	if _, err := store.Save("ada", Preset{Name: "Reviewer", Harness: "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save("ada", Preset{Name: "reviewer", Harness: "codex"}); err == nil {
		t.Error("a second preset took a name that was in use")
	}
	if _, err := store.Save("", Preset{Name: "x", Harness: "claude"}); err == nil {
		t.Error("a preset was kept for nobody")
	}
}

func TestOnlyThePersonWhoSavedAPresetCanRemoveIt(t *testing.T) {
	store, _ := Open(t.TempDir())
	saved, _ := store.Save("ada", Preset{Name: "Reviewer", Harness: "claude"})
	if removed, _ := store.Delete("bob", saved.ID); removed {
		t.Error("bob removed ada's preset")
	}
	if removed, err := store.Delete("ada", saved.ID); !removed || err != nil {
		t.Errorf("ada could not remove her own: %v %v", removed, err)
	}
	if len(store.List("ada")) != 0 {
		t.Error("the preset is still there")
	}
}

func TestPresetsAreKeptAcrossReopening(t *testing.T) {
	dir := t.TempDir()
	first, _ := Open(dir)
	first.Save("ada", Preset{Name: "One", Harness: "claude"})
	first.Save("ada", Preset{Name: "Two", Harness: "claude"})
	second, _ := Open(dir)
	got := second.List("ada")
	if len(got) != 2 || got[0].Name != "Two" {
		t.Errorf("after reopening, newest first: %+v", got)
	}
}
