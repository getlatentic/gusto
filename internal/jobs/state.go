// Package jobs is the set of job directories a server knows about, and what
// each one is doing.
//
// The state lives in the job directory beside the evidence, so a job is still
// the whole story of itself: copy the directory to another machine and nothing
// is left behind in a database. The index in memory is a cache of what is on
// disk, rebuilt by reading it.
package jobs

import (
	"encoding/json"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/job"
)

type State string

const (
	Queued      State = "queued"
	Running     State = "running"
	Passed      State = "passed"
	Failed      State = "failed"
	Interrupted State = "interrupted"
)

// Done reports whether a job will change no further without being asked again.
// String is the word for the state, so a message can say it.
func (s State) String() string { return string(s) }

func (s State) Done() bool {
	return s == Passed || s == Failed || s == Interrupted
}

// Record is what a person needs to see in a list, and what the queue needs to
// pick the next job. Everything else is in the directory.
type Record struct {
	ID      string `json:"id"`
	State   State  `json:"state"`
	Brief   string `json:"brief"`
	Repo    string `json:"repo"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	// Instance and Thread are set when this job is a turn in a conversation.
	// The instance is what serialises them: its threads share one workspace, so
	// two of its turns must never run at once.
	Instance string `json:"instance,omitempty"`
	Thread   string `json:"thread,omitempty"`
	Asked    string `json:"asked"`
	// Key is the request id the sender gave, so that the same send made twice is one job. It belongs to the asker.
	Key string `json:"requestId,omitempty"`
	// StoppedBy is who asked for the job to be stopped. A job with it ends interrupted, whatever its harness said on the way out.
	StoppedBy string `json:"stoppedBy,omitempty"`
	// Team is who the job counts against, for the team limit.
	Team string `json:"team,omitempty"`
	// Seq is the order this job arrived in. Queued is a second, and two jobs
	// submitted in one second are two messages a person sent in order, so the
	// clock alone cannot say which of them goes first.
	Seq     int64 `json:"seq,omitempty"`
	Exit    int   `json:"exit"`
	Queued  int64 `json:"queuedAt"`
	Started int64 `json:"startedAt,omitempty"`
	Ended   int64 `json:"endedAt,omitempty"`
}

func (r Record) Dir(root string) job.Dir { return job.Dir(root + "/" + r.ID) }

func Now() int64 { return time.Now().Unix() }

func read(dir job.Dir) (Record, error) {
	text, err := os.ReadFile(dir.Path() + "/state.json")
	if err != nil {
		return Record{}, err
	}
	var record Record
	err = json.Unmarshal(text, &record)
	return record, err
}

func write(dir job.Dir, record Record) error {
	text, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dir.Path()+"/state.json", append(text, '\n'), 0o644)
}
