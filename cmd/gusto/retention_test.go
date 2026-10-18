package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// captured is what fn writes to standard output.
func captured(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	was := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	code := fn()
	write.Close()
	os.Stdout = was
	text, _ := io.ReadAll(read)
	return string(text), code
}

func TestTheRetentionCommandListsWhatWouldGoAndRemovesNothing(t *testing.T) {
	root := t.TempDir()
	index, err := jobs.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ended := time.Now().AddDate(0, 0, -30).Unix()
	if err := os.MkdirAll(filepath.Join(root, "old", "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "old", "workspace", "f"), []byte(strings.Repeat("x", 300)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(jobs.Record{ID: "old", State: jobs.Passed, Brief: "old", Ended: ended, Queued: ended}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), []byte(`{"workspaceDays":{"passed":7}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := captured(t, func() int { return retentionCommand([]string{"-tasks", root, "-dir", dir}) })
	if code != 0 || !strings.Contains(out, "old") || !strings.Contains(out, "workspace") ||
		!strings.Contains(out, "1 to remove, freeing 300 bytes") || !strings.Contains(out, "Nothing was removed") {
		t.Fatalf("code %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "old", "workspace", "f")); err != nil {
		t.Fatal("the command removed a workspace")
	}
	if _, code := captured(t, func() int { return retentionCommand([]string{"-tasks", root}) }); code != 2 {
		t.Errorf("no retention directory gave %d", code)
	}
}
