package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/apitoken"
	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/datadir"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/preset"
	"github.com/getlatentic/haas/internal/retention"
	"github.com/getlatentic/haas/internal/setup"
)

// installed points every setting at its own directory and returns them.
func installed(t *testing.T) map[datadir.Store]string {
	t.Helper()
	root := t.TempDir()
	for name, path := range map[string]string{
		"HAAS_JOBS": "jobs", "HAAS_SESSIONS": "sessions", "HAAS_SETUPS": "setups", "HAAS_COMPARISONS": "comparisons", "HAAS_INTEGRATIONS": "integrations", "HAAS_PRESETS": "presets",
		"HAAS_CHANGES": "changes/changes.jsonl", "HAAS_API_TOKENS": "tokens/tokens.json", "HAAS_RETENTION": "retention",
	} {
		t.Setenv(name, filepath.Join(root, path))
	}
	return dataDirs()
}

// Every store reads a directory that carries the stamp, so a release that
// predates stamps still runs on data a newer one has stamped, which is what a
// rollback needs.
func TestEveryStoreOpensADirectoryThatCarriesTheStamp(t *testing.T) {
	dirs := installed(t)
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	for store, dir := range dirs {
		if _, stamped, err := datadir.Read(dir); err != nil || !stamped {
			t.Fatalf("%s was not stamped: %v", store, err)
		}
	}
	if _, err := jobs.Open(dirs[datadir.Jobs]); err != nil {
		t.Errorf("jobs: %v", err)
	}
	if _, err := instance.Open(dirs[datadir.Sessions]); err != nil {
		t.Errorf("sessions: %v", err)
	}
	if _, err := setup.Open(dirs[datadir.Setups]); err != nil {
		t.Errorf("setups: %v", err)
	}
	board, err := compare.OpenBoard(dirs[datadir.Comparisons])
	if err != nil {
		t.Fatalf("comparisons: %v", err)
	}
	if len(board.List()) != 0 {
		t.Error("the stamp was read as a comparison")
	}
	if _, err := apitoken.Open(filepath.Join(dirs[datadir.Tokens], "tokens.json")); err != nil {
		t.Errorf("tokens: %v", err)
	}
	if _, err := retention.OpenHolds(dirs[datadir.Retention]).Read(); err != nil {
		t.Errorf("retention: %v", err)
	}
	if store, err := preset.Open(dirs[datadir.Presets]); err != nil || len(store.List("ada")) != 0 {
		t.Errorf("presets: %v", err)
	}
	if listed, _ := jobs.Scan(dirs[datadir.Jobs]); len(listed) != 0 {
		t.Error("the stamp was read as a job")
	}
}

func TestStartingOnNewerDataChangesNothingAnywhere(t *testing.T) {
	dirs := installed(t)
	if err := os.MkdirAll(dirs[datadir.Sessions], 0o755); err != nil {
		t.Fatal(err)
	}
	newer := `{"format": 9, "release": "v9.9.9", "at": 1}` + "\n"
	if err := os.WriteFile(filepath.Join(dirs[datadir.Sessions], datadir.StampFile), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ready()
	if err == nil || !strings.Contains(err.Error(), "format 9") || !strings.Contains(err.Error(), "v9.9.9") {
		t.Fatalf("%v", err)
	}
	for store, dir := range dirs {
		if store == datadir.Sessions {
			continue
		}
		if _, stamped, _ := datadir.Read(dir); stamped {
			t.Errorf("%s was stamped although another store refused", store)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dirs[datadir.Sessions], datadir.StampFile)); string(got) != newer {
		t.Error("a refusal rewrote the stamp")
	}
	if dataCheck() != 1 {
		t.Error("check did not say the server would refuse")
	}
}

func TestDataRollbackNamesAReleaseAndRefusesAnUnknownOne(t *testing.T) {
	dirs := installed(t)
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	if code := dataRollback([]string{"-to", "v0.1.0-rc.1"}); code != 0 {
		t.Fatalf("rolling back to the release that wrote format 1 gave %d", code)
	}
	if stamp, _, _ := datadir.Read(dirs[datadir.Jobs]); stamp.Format != 1 {
		t.Errorf("stamp %+v", stamp)
	}
	if code := dataRollback([]string{"-to", "v9"}); code == 0 {
		t.Error("an unknown release was accepted")
	}
	if code := dataRollback(nil); code != 2 {
		t.Errorf("no release gave %d", code)
	}
}
