package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/pkg/job"
)

// StopFile is where a person who stopped a task says so, in the task's directory beside its spec. The server writes it before it
// tells the run to stop, and the run reads it when it ends, so the record can say who. A run that was stopped by a signal alone,
// as a container is when its server shuts down, finds no file and says so.
func StopFile(dir job.Dir) string { return filepath.Join(dir.Path(), "stop.json") }

type stopNote struct {
	By string `json:"by"`
	At int64  `json:"at"`
}

// NoteStop writes who stopped the task and when.
func NoteStop(dir job.Dir, by string, at int64) error {
	text, err := json.Marshal(stopNote{By: by, At: at})
	if err != nil {
		return err
	}
	return os.WriteFile(StopFile(dir), text, 0o644)
}

// stoppedBy is who stopped the run, or nobody when it was not stopped. A run whose context ended without its budget running
// out was told to stop: by the person the stop file names, or by a signal when it names nobody.
func stoppedBy(ctx context.Context, dir job.Dir) string {
	if ctx.Err() != context.Canceled {
		return ""
	}
	text, err := os.ReadFile(StopFile(dir))
	if err != nil {
		return "a signal"
	}
	var note stopNote
	if json.Unmarshal(text, &note) != nil || note.By == "" {
		return "a signal"
	}
	return note.By
}
