package web

import (
	"encoding/json"
	"os"

	"github.com/getlatentic/gusto/pkg/task"
)

// fullHome is what a turn's record says when the person's kept home is why the turn did not run or was stopped, read from
// out/home.json, which the container backend writes.
type fullHome struct {
	// Outcome is "refused" for a turn that did not start and "stopped" for one the home outgrew while it ran.
	Outcome     string
	Used, Limit string
}

// homeFullOf is the turn's note about a full home, or nil when its home was not why it ended.
func homeFullOf(dir task.Dir) *fullHome {
	text, err := os.ReadFile(dir.OutFile("home.json"))
	if err != nil {
		return nil
	}
	var note struct {
		Outcome string `json:"outcome"`
		Used    int64  `json:"used"`
		Limit   int64  `json:"limit"`
	}
	if json.Unmarshal(text, &note) != nil || note.Limit <= 0 {
		return nil
	}
	return &fullHome{Outcome: note.Outcome, Used: homeSize(note.Used), Limit: homeSize(note.Limit)}
}
