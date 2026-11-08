package roomwork

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var roomy = Limits{Bytes: 64 << 20, Files: 10000}

// aRoom is a room whose folder holds the files given, and a hub to work on it with.
func aRoom(t *testing.T, files map[string]string) (*Hub, Room) {
	t.Helper()
	base := t.TempDir()
	in := Room{Folder: filepath.Join(base, "workspace"), Meta: filepath.Join(base, "merge")}
	if err := os.MkdirAll(in.Folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		write(t, in.Folder, name, text)
	}
	return NewHub(roomy), in
}

func write(t *testing.T, folder, name, text string) {
	t.Helper()
	path := filepath.Join(folder, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// begin starts a turn for an agent on a fresh copy.
func begin(t *testing.T, hub *Hub, in Room, agent string) *Turn {
	t.Helper()
	turn, err := hub.Begin(context.Background(), in, filepath.Join(t.TempDir(), "workspace"), agent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(turn.Close)
	return turn
}

func merge(t *testing.T, turn *Turn) Outcome {
	t.Helper()
	outcome, err := turn.Merge(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func read(t *testing.T, folder, name string) string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(text)
}

func missing(folder, name string) bool {
	_, err := os.Lstat(filepath.Join(folder, filepath.FromSlash(name)))
	return os.IsNotExist(err)
}

// listing is every file in a folder, with its text, so a test can compare whole folders.
func listing(t *testing.T, folder string) string {
	t.Helper()
	found, err := scan(folder)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for name := range found {
		lines = append(lines, name+"="+read(t, folder, name))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func noStaging(t *testing.T, in Room) {
	t.Helper()
	for _, left := range []string{stageDir(in), backupDir(in), journalOf(in)} {
		if exists(left) {
			t.Errorf("%s was left behind", left)
		}
	}
}
