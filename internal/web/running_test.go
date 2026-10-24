package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// startedTheOldWay writes an instance the way the two-step start left one on
// disk: a computer with a workspace, and no conversation opened on it.
func startedTheOldWay(t *testing.T, root, id string) {
	t.Helper()
	dir := filepath.Join(root, id)
	for _, each := range []string{"workspace", "state", "threads"} {
		if err := os.MkdirAll(filepath.Join(dir, each), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	text, err := json.Marshal(instance.Instance{ID: id, Harness: "fixture", Started: 1790000000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "instance.json"), text, 0o644); err != nil {
		t.Fatal(err)
	}
}

// openOver is a server over instances that were already on disk before it
// started, which is the only way an instance from the old start reaches it.
func openOver(t *testing.T, instances string) *site {
	t.Helper()
	index, err := jobs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := instance.Open(instances)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, store, nil, password, &Identity{}, offered(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &site{handler: server.Handler(), index: index}
}
