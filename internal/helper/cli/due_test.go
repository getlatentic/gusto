package cli

import (
	"slices"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
)

func ended() chan struct{} { done := make(chan struct{}); close(done); return done }

// A folder registered again at a new path stops its engine, which stays among the running until it has ended, and is then
// started again at once, at the new path.
func TestAFolderRegisteredAtANewPathRestartsItsEngineOnceTheOldOneHasEnded(t *testing.T) {
	now := time.Now()
	agents := map[string]helper.Instance{"k": {ID: "a1", Folder: "k"}}
	paths := map[string]string{"k": "/work/new"}
	going := &kept{instance: "a1", path: "/work/old", cancel: func() {}, done: make(chan struct{})}
	running := map[string]*kept{"k": going}

	stop, start := due(running, paths, agents, now)
	if !slices.Equal(stop, []string{"k"}) || len(start) != 0 {
		t.Fatalf("a folder whose path changed: stop %v, start %v", stop, start)
	}
	going.cancelled = true
	if stop, start := due(running, paths, agents, now); len(stop) != 0 || len(start) != 0 {
		t.Fatalf("an engine still ending was stopped again or had a second started beside it: stop %v, start %v", stop, start)
	}
	going.done, going.stopped = ended(), now
	if _, start := due(running, paths, agents, now); !slices.Equal(start, []string{"k"}) {
		t.Errorf("the ended engine was not started again at once: %v", start)
	}
}

// An engine that stopped on its own is started again only after folderRetry, and one whose agent changed is stopped.
func TestAnEngineThatStoppedOnItsOwnWaitsAndOneWhoseAgentChangedStops(t *testing.T) {
	now := time.Now()
	agents := map[string]helper.Instance{"k": {ID: "a2", Folder: "k"}, "j": {ID: "b1", Folder: "j"}}
	paths := map[string]string{"k": "/work/k", "j": "/work/j"}
	running := map[string]*kept{
		"k": {instance: "a1", path: "/work/k", cancel: func() {}, done: make(chan struct{})},
		"j": {instance: "b1", path: "/work/j", cancel: func() {}, done: ended(), stopped: now.Add(-time.Second)},
	}
	stop, start := due(running, paths, agents, now)
	if !slices.Equal(stop, []string{"k"}) || len(start) != 0 {
		t.Errorf("stop %v, start %v", stop, start)
	}
	if _, start := due(running, paths, agents, now.Add(folderRetry)); !slices.Contains(start, "j") {
		t.Errorf("an engine that stopped on its own was not started again after folderRetry: %v", start)
	}
}
