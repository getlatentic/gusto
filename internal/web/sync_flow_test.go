package web

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/helper"
	"github.com/getlatentic/haas/internal/helper/mirror"
)

// A person's folder and an agent's folder on the real server are kept the same, and when both sides change a file neither
// version is lost. The "agent" writes into its folder on the server as the agent does, by the file system.
func TestAPersonsFolderAndAnAgentsFolderOnTheRealServerAreKeptTheSameAndConflictsLoseNothing(t *testing.T) {
	here := withTokens(t)
	where, _ := started(t, here.as(t, alice))
	id := strings.TrimPrefix(where, "/instances/")
	cloudRoot := here.server.instances.Workspace(id)
	server := httptest.NewServer(here.handler)
	t.Cleanup(server.Close)
	client := helper.Client{Server: server.URL, Token: signInADevice(t, here, alice)}

	work := t.TempDir()
	state, err := mirror.OpenState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine := func() *mirror.Engine {
		state, _ = mirror.OpenState(filepath.Join(filepath.Dir(work), "state-"+filepath.Base(work)+".json"))
		return &mirror.Engine{Root: work, Remote: mirror.Cloud{Client: client, Instance: id}, State: state, Ignore: mirror.NewIgnore(),
			Now: func() time.Time { return time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC) }, Log: func(string, ...any) {}}
	}
	run := func() *mirror.Report {
		t.Helper()
		report, err := engine().Once(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Errors) > 0 {
			t.Fatalf("%v", report.Errors)
		}
		return report
	}
	agentWrites := func(name, text string) {
		full := filepath.Join(cloudRoot, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(text), 0o644)
		later := time.Now().Add(time.Minute)
		os.Chtimes(full, later, later)
	}
	personWrites := func(name, text string) {
		full := filepath.Join(work, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(text), 0o644)
		later := time.Now().Add(2 * time.Minute)
		os.Chtimes(full, later, later)
	}
	read := func(root, name string) string {
		text, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		return string(text)
	}

	agentWrites("src/main.go", "package main")
	agentWrites("node_modules/dep/index.js", "never crosses")
	personWrites("README.md", "# written here")
	run()
	if read(work, "src/main.go") != "package main" || read(cloudRoot, "README.md") != "# written here" {
		t.Fatalf("the first run did not carry both ways: %q %q", read(work, "src/main.go"), read(cloudRoot, "README.md"))
	}
	if _, err := os.Stat(filepath.Join(work, "node_modules")); err == nil {
		t.Error("an ignored folder came down")
	}
	if again := run(); again.Done[mirror.Pull]+again.Done[mirror.Push]+again.Done[mirror.Conflict] != 0 {
		t.Errorf("a run with nothing changed did %+v", again.Done)
	}

	agentWrites("README.md", "# the agent's edit")
	personWrites("README.md", "# the person's edit")
	report := run()
	if len(report.Conflicts) != 1 {
		t.Fatalf("%+v", report)
	}
	copyName := report.Conflicts[0]
	if read(work, "README.md") != "# the person's edit" || read(work, copyName) != "# the agent's edit" {
		t.Errorf("the person's folder: %q and %q", read(work, "README.md"), read(work, copyName))
	}
	if read(cloudRoot, "README.md") != "# the person's edit" || read(cloudRoot, copyName) != "# the agent's edit" {
		t.Errorf("the agent's folder: %q and %q: the agent has to be able to find both", read(cloudRoot, "README.md"), read(cloudRoot, copyName))
	}
	if again := run(); again.Done[mirror.Pull]+again.Done[mirror.Push]+again.Done[mirror.Conflict] != 0 {
		t.Errorf("after a conflict the sides still differ: %+v", again.Done)
	}

	os.Remove(filepath.Join(work, "src/main.go"))
	run()
	if _, err := os.Stat(filepath.Join(cloudRoot, "src/main.go")); err == nil {
		t.Error("a file deleted here is still in the agent's folder")
	}
	if entries, _ := filepath.Glob(filepath.Join(cloudRoot, ".haas-write-*")); len(entries) != 0 {
		t.Errorf("a write left %v", entries)
	}
}
