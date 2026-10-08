package integration

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "integrations"))
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	return store
}

func versionOf(label string, _ map[string]string) map[string]string {
	files := map[string]string{
		"plugin.json":               `{` + pluginSchema + `, "name": "reports", "version": "` + label + `"}`,
		"skills/summarize/SKILL.md": skillText("summarize", "Summarise a report."),
		"mcp.json":                  `{` + mcpHeader + `, "mcpServers": {"api": {"type": "streamable-http", "url": "https://api.example.com/mcp"}}}`,
	}
	return files
}

func with(files map[string]string, more map[string]string) map[string]string {
	out := map[string]string{}
	for name, text := range files {
		out[name] = text
	}
	for name, text := range more {
		out[name] = text
	}
	return out
}

var git = Source{Kind: "git", Address: "https://example.com/reports.git", Ref: "main", Commit: "abc123"}

func TestAnImportedIntegrationIsOffUntilAnAdministratorTurnsItOn(t *testing.T) {
	store := openStore(t)
	held, version, err := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if held.On != "" || version.Source.Commit != "abc123" || version.By != "ada" || len(version.Plugin.Skills) != 1 {
		t.Fatalf("%+v %+v", held, version)
	}
	if on, _ := store.Active(); len(on) != 0 {
		t.Fatalf("an integration nobody turned on is active: %+v", on)
	}
	if _, err := store.Enable("reports", version.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	on, err := store.Active()
	if err != nil || len(on) != 1 || on[0].Name != "reports" {
		t.Fatalf("%+v %v", on, err)
	}
	if _, err := os.Stat(filepath.Join(on[0].Dir, "skills", "summarize", "SKILL.md")); err != nil {
		t.Errorf("the version's files are not kept: %v", err)
	}
}

func TestANewVersionDoesNotReplaceTheOneThatIsOnAndTheOldOneCanBePutBack(t *testing.T) {
	store := openStore(t)
	_, one, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	store.Enable("reports", one.ID, "ada")
	held, two, err := store.Import(folder(t, with(versionOf("1.1.0", nil), map[string]string{"skills/summarize/SKILL.md": skillText("summarize", "Summarise it differently.")})), git, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if held.On != one.ID || len(held.Versions) != 2 {
		t.Fatalf("importing a new version changed what is on: %+v", held)
	}
	if on, _ := store.Active(); on[0].Version.ID != one.ID {
		t.Fatal("people got the new version before it was turned on")
	}
	held, _ = store.Enable("reports", two.ID, "bo")
	if held.On != two.ID || held.Before != one.ID {
		t.Fatalf("%+v", held)
	}
	held, err = store.Restore("reports", "bo")
	if err != nil || held.On != one.ID || held.Before != two.ID {
		t.Fatalf("restore: %+v %v", held, err)
	}
	held, _ = store.Restore("reports", "bo")
	if held.On != two.ID {
		t.Error("a second restore puts the newer one back")
	}
	var actions []string
	for _, event := range held.History {
		actions = append(actions, event.Action+":"+event.By)
	}
	want := []string{"imported:ada", "enabled:ada", "imported:ada", "enabled:bo", "restored:bo", "restored:bo"}
	if !reflect.DeepEqual(actions, want) {
		t.Errorf("history = %v, want %v", actions, want)
	}
}

func TestTurningOffKeepsEveryVersionAndRestoreTurnsItBackOn(t *testing.T) {
	store := openStore(t)
	_, one, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	store.Enable("reports", one.ID, "ada")
	held, _ := store.Disable("reports", "ada")
	if held.On != "" || len(held.Versions) != 1 {
		t.Fatalf("%+v", held)
	}
	if on, _ := store.Active(); len(on) != 0 {
		t.Fatal("a disabled integration is active")
	}
	if held, err := store.Restore("reports", "ada"); err != nil || held.On != one.ID {
		t.Fatalf("%+v %v", held, err)
	}
}

func TestTheSameFilesImportedAgainAreTheSameVersion(t *testing.T) {
	store := openStore(t)
	_, first, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	held, again, err := store.Import(folder(t, versionOf("1.0.0", nil)), Source{Kind: "upload"}, "bo")
	if err != nil || again.ID != first.ID || len(held.Versions) != 1 || len(held.History) != 1 {
		t.Fatalf("%+v %v", held, err)
	}
}

func TestComparingTwoVersionsSaysWhatAnAdministratorWouldBeTurningOn(t *testing.T) {
	store := openStore(t)
	_, one, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	two := with(versionOf("1.1.0", nil), map[string]string{
		"skills/summarize/SKILL.md": skillText("summarize", "Changed."),
		"skills/audit/SKILL.md":     skillText("audit", "New."),
		"mcp.json": `{` + mcpHeader + `, "mcpServers": {
			"api": {"type": "streamable-http", "url": "https://elsewhere.example.com/mcp"},
			"extra": {"type": "stdio", "command": "extra"}}}`,
	})
	held, second, _ := store.Import(folder(t, two), git, "ada")
	changes, err := held.Compare(one.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := Changes{SkillsAdded: []string{"audit"}, SkillsChanged: []string{"summarize"}, ServersAdded: []string{"extra"}, ServersChanged: []string{"api"}, Manifest: []string{"version"}}
	if !reflect.DeepEqual(changes, want) {
		t.Errorf("changes = %+v, want %+v", changes, want)
	}
	if same, _ := held.Compare(one.ID, one.ID); !same.None() {
		t.Errorf("a version differs from itself: %+v", same)
	}
	removed := with(versionOf("1.2.0", nil), nil)
	delete(removed, "skills/summarize/SKILL.md")
	delete(removed, "mcp.json")
	held, third, _ := store.Import(folder(t, removed), git, "ada")
	gone, _ := held.Compare(one.ID, third.ID)
	if !reflect.DeepEqual(gone.SkillsRemoved, []string{"summarize"}) || !reflect.DeepEqual(gone.ServersRemoved, []string{"api"}) {
		t.Errorf("%+v", gone)
	}
}

func TestAPluginThatCannotBeTakenIsNotKeptAndNeitherIsAFileThatLeavesIt(t *testing.T) {
	store := openStore(t)
	if _, _, err := store.Import(folder(t, map[string]string{"skills/a/SKILL.md": skillText("a", "d")}), git, "ada"); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v", err)
	}
	if all, _ := store.List(); len(all) != 0 {
		t.Fatalf("a rejected plugin was kept: %+v", all)
	}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("hunter2"), 0o600)
	_, version, err := store.Import(folder(t, with(versionOf("1.0.0", nil), map[string]string{"notes/out.txt": "-> " + secret, "notes/in.txt": "-> ../plugin.json", ".git/config": "x"})), git, "ada")
	if err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(store.folder("reports", version.ID))
	if _, err := os.Lstat(filepath.Join(kept, "notes", "out.txt")); err == nil {
		t.Error("a file outside the plugin was copied in")
	}
	if info, err := os.Lstat(filepath.Join(kept, "notes", "in.txt")); err != nil || !info.Mode().IsRegular() {
		t.Errorf("a link to a file in the plugin is not kept as the file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(kept, ".git")); err == nil {
		t.Error("version control's folder was kept")
	}
}

func TestAFileKeepsItsRunnableBitAndTheKeptFilesAreWhatWasImported(t *testing.T) {
	store := openStore(t)
	source := folder(t, with(versionOf("1.0.0", nil), map[string]string{"skills/summarize/scripts/run.sh": "#!/bin/sh\n"}))
	os.Chmod(filepath.Join(source, "skills/summarize/scripts/run.sh"), 0o755)
	_, version, _ := store.Import(source, git, "ada")
	kept := store.folder("reports", version.ID)
	if info, err := os.Stat(filepath.Join(kept, "skills/summarize/scripts/run.sh")); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("a script lost its runnable bit: %v", err)
	}
	got, err := digest(kept, kept)
	if err != nil || got != version.Digest {
		t.Errorf("what is kept has digest %s, and %s was imported (%v)", got, version.Digest, err)
	}
}

func TestTheStoreIsTheSameAfterARestartAndADamagedFileIsAnErrorNotAnEmptyList(t *testing.T) {
	store := openStore(t)
	_, version, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	store.Enable("reports", version.ID, "ada")
	again, _ := Open(store.root)
	if on, err := again.Active(); err != nil || len(on) != 1 {
		t.Fatalf("%+v %v", on, err)
	}
	os.WriteFile(store.file("reports"), []byte("{not json"), 0o600)
	if _, err := again.List(); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("a damaged record was taken for none: %v", err)
	}
	if _, err := again.Active(); err == nil {
		t.Error("a damaged record means nothing is on")
	}
}

func TestAnUnknownIntegrationOrVersionIsRefusedAndANameThatClimbsIsNotLooked(t *testing.T) {
	store := openStore(t)
	_, version, _ := store.Import(folder(t, versionOf("1.0.0", nil)), git, "ada")
	if _, err := store.Enable("reports", "nope", "ada"); !errors.Is(err, ErrNoVersion) {
		t.Errorf("err = %v", err)
	}
	if _, err := store.Enable("other", version.ID, "ada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	for _, name := range []string{"../reports", "reports/../../x", "", "."} {
		if _, err := store.Get(name); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
	if _, err := store.Restore("reports", "ada"); err == nil {
		t.Error("there is nothing to restore")
	}
}
