package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

func memory() Servers {
	return Servers{"memory": {Name: "memory", Command: "mcp-server-memory", Env: map[string]string{"TOKEN": "s3cret"}}}
}

func TestASetupKeepsItsSkillsAndServers(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Setup{Name: "reviewer", Servers: []string{"memory"}}, memory()); err != nil {
		t.Fatal(err)
	}
	if err := store.Write("reviewer", Skill{Name: "review", Description: "Use when asked: \"review this\".", Body: "Read the diff first."}); err != nil {
		t.Fatal(err)
	}
	held, found := store.Get("reviewer")
	if !found || len(held.Skills) != 1 || held.Servers[0] != "memory" {
		t.Fatalf("held %+v", held)
	}
	skill := held.Skills[0]
	if skill.Name != "review" || skill.Description != `Use when asked: "review this".` || skill.Body != "Read the diff first." {
		t.Errorf("the skill came back as %+v", skill)
	}
	if err := store.Drop("reviewer", "review"); err != nil {
		t.Fatal(err)
	}
	if held, _ := store.Get("reviewer"); len(held.Skills) != 0 {
		t.Error("a dropped skill is still there")
	}
}

func TestASetupNamesOnlyServersTheDeploymentOffers(t *testing.T) {
	store, _ := Open(t.TempDir())
	if err := store.Save(Setup{Name: "wide", Servers: []string{"shell-anywhere"}}, memory()); err == nil {
		t.Error("a setup named a server nobody offered")
	}
	if err := store.Save(Setup{Name: "../escape"}, memory()); err == nil {
		t.Error("a setup was named with a path")
	}
}

// A job carries its skills and its servers' definitions beside it, and names
// them in job.json. The definitions stay out of job.json, which anyone with
// the run can read, and the file holding them is its owner's alone.
func TestAJobCarriesTheSetupWithoutPuttingDefinitionsInItsEvidence(t *testing.T) {
	store, _ := Open(t.TempDir())
	if err := store.Save(Setup{Name: "reviewer", Servers: []string{"memory"}}, memory()); err != nil {
		t.Fatal(err)
	}
	if err := store.Write("reviewer", Skill{Name: "review", Description: "d", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	carried, err := store.Carry("reviewer", memory(), dir)
	if err != nil {
		t.Fatal(err)
	}
	named, _ := json.Marshal(carried)
	if strings.Contains(string(named), "s3cret") || carried.Skills[0] != "review" || carried.Servers[0] != "memory" {
		t.Errorf("job.json would carry %s", named)
	}
	if _, err := os.Stat(filepath.Join(dir.SetupDir(), "skills", "review", "SKILL.md")); err != nil {
		t.Error("the skill did not travel with the job")
	}
	info, err := os.Stat(filepath.Join(dir.SetupDir(), "servers.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the server definitions are not the owner's alone: %v", err)
	}
}

func TestAServerFileNamesACommandOrAnAddress(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "memory.json"), []byte(`{"command": "mcp-server-memory"}`), 0o644)
	offered, err := OpenServers(dir)
	if err != nil || offered["memory"].Command != "mcp-server-memory" || offered["memory"].Name != "memory" {
		t.Fatalf("offered %+v, %v", offered, err)
	}
	os.WriteFile(filepath.Join(dir, "neither.json"), []byte(`{}`), 0o644)
	if _, err := OpenServers(dir); err == nil {
		t.Error("a server with no command and no address was offered")
	}
}
