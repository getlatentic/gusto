package retention

import (
	"path/filepath"

	"github.com/getlatentic/gusto/internal/tasks"
)

// Usage is what the tasks take on disk, and how far back they go.
type Usage struct {
	Bytes      int64
	Tasks      int
	Workspaces int64
	// Oldest is the finished task kept longest, or nil when none has finished.
	Oldest *Oldest
}

// Oldest is the finished task that has been kept longest.
type Oldest struct {
	ID    string
	Ended int64
}

// Measure adds up the disk the tasks under root take. It walks every task's
// directory, so a caller that draws it on a page keeps the answer for a while.
func Measure(root string, all []tasks.Record) Usage {
	var used Usage
	for _, record := range all {
		dir := filepath.Join(root, record.ID)
		used.Tasks++
		used.Bytes += size(dir)
		used.Workspaces += size(filepath.Join(dir, "workspace"))
		if !record.State.Done() {
			continue
		}
		ended := record.Ended
		if ended == 0 {
			ended = record.Queued
		}
		if used.Oldest == nil || ended < used.Oldest.Ended {
			used.Oldest = &Oldest{ID: record.ID, Ended: ended}
		}
	}
	return used
}
