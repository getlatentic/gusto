package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// folderHolds records what a server asked the room's merges to stay out of, and what it said it changed.
type folderHolds struct {
	paths   [][]string
	changed []bool
	refuse  error
}

func (h *folderHolds) hold(_ context.Context, _ string, paths []string) (func(bool), error) {
	if h.refuse != nil {
		return nil, h.refuse
	}
	h.paths = append(h.paths, paths)
	return func(changed bool) { h.changed = append(h.changed, changed) }, nil
}

func TestAPersonsWriteOrDeleteHoldsTheFolderAgainstAMergeAndSaysWhatChanged(t *testing.T) {
	here, id, ada, _ := aFolder(t)
	held := &folderHolds{}
	here.server.UseRoomHold(held.hold)

	here.put(ada, id, "docs/plan.md", "v1", absent)
	here.put(ada, id, "docs/plan.md", "v2", sum("stale"))
	here.remove(ada, id, "docs/plan.md", sum("v1"))

	if len(held.paths) != 3 || held.paths[0][0] != "docs/plan.md" {
		t.Fatalf("held %v", held.paths)
	}
	if want := []bool{true, false, true}; len(held.changed) != 3 || held.changed[0] != want[0] || held.changed[1] != want[1] || held.changed[2] != want[2] {
		t.Errorf("said changed %v, want %v", held.changed, want)
	}
}

func TestAWriteThatCannotHoldTheFolderIsRefusedAndWritesNothing(t *testing.T) {
	here, id, ada, root := aFolder(t)
	here.server.UseRoomHold((&folderHolds{refuse: errors.New("a merge is taking too long")}).hold)
	if got := here.put(ada, id, "plan.md", "v1", absent); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if _, err := os.Lstat(filepath.Join(root, "plan.md")); err == nil {
		t.Error("a write that could not hold the folder wrote the file")
	}
}
