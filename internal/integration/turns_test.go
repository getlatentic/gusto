package integration

import "testing"

func TestTurningOffWhatIsOffChangesNothingAndWritesNothingDown(t *testing.T) {
	store := openStore(t)
	store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	before, _ := store.Get("reports")
	if _, err := store.Disable("reports", "ada"); err == nil {
		t.Error("turning off an integration that is off was taken as a change")
	}
	if after, _ := store.Get("reports"); len(after.History) != len(before.History) {
		t.Errorf("turning off what was off was written down: %+v", after.History[len(after.History)-1])
	}
}

func TestProgramsAllowedBeforeATurnOffAreAskedForAgainWhenItComesBack(t *testing.T) {
	store := openStore(t)
	_, one, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	store.Enable("reports", one.ID, "ada")
	store.AllowPrograms("reports", one.ID, "ada")
	store.Disable("reports", "ada")
	held, err := store.Enable("reports", one.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if held.ProgramsRun() {
		t.Error("an integration turned off and on again ran its programs without anybody allowing them again")
	}
}

func TestEachChangeSaysWhatItReplaced(t *testing.T) {
	store := openStore(t)
	_, one, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	_, two, _ := store.Import(folder(t, versionOf("1.1.0", nil)), git, "ada")
	store.Enable("reports", one.ID, "ada")
	held, _ := store.Enable("reports", two.ID, "ada")
	if was := held.History[len(held.History)-1].Was; was != one.ID {
		t.Errorf("turning on the newer version says it replaced %q", was)
	}
	held, _ = store.SetFor("reports", []string{"*@acme.com"}, "ada")
	held, _ = store.SetFor("reports", nil, "ada")
	if was := held.History[len(held.History)-1].Was; was != "*@acme.com" {
		t.Errorf("scoping it again says it replaced %q", was)
	}
}
