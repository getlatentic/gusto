// Package jobs is the set of task directories a server knows about, and what
// each one is doing.
//
// The state lives in the task directory beside the record, so a task is still
// the whole story of itself: copy the directory to another machine and nothing
// is left behind in a database. The index in memory is a cache of what is on
// disk, rebuilt by reading it.
package jobs

import (
	"encoding/json"
	"os"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

type State string

const (
	Queued      State = "queued"
	Running     State = "running"
	Passed      State = "passed"
	Failed      State = "failed"
	Interrupted State = "interrupted"
)

// Done reports whether a task will change no further without being asked again.
// String is the word for the state, so a message can say it.
func (s State) String() string { return string(s) }

func (s State) Done() bool {
	return s == Passed || s == Failed || s == Interrupted
}

// Record is what a person needs to see in a list, and what the queue needs to
// pick the next task. Everything else is in the directory.
type Record struct {
	ID      string `json:"id"`
	State   State  `json:"state"`
	Brief   string `json:"brief"`
	Repo    string `json:"repo"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	// Instance and Thread are set when this task is a turn in a conversation. See Lane for which turns wait for each other.
	Instance string `json:"instance,omitempty"`
	Thread   string `json:"thread,omitempty"`
	// Agent is the room agent that took this turn, HandedBy the agent whose reply asked for it, and Hops how many agents handed
	// it on before it: a turn somebody asked for is 0.
	Agent    string `json:"agent,omitempty"`
	HandedBy string `json:"handedBy,omitempty"`
	// Answers is the first turn of the message this turn also answers, when one message mentioned several agents.
	Answers string `json:"answers,omitempty"`
	Hops    int    `json:"hops,omitempty"`
	Asked   string `json:"asked"`
	// Key is the request id the sender gave, so that the same send made twice is one task. It belongs to the asker.
	Key string `json:"requestId,omitempty"`
	// StoppedBy is who asked for the task to be stopped. A task with it ends interrupted, whatever its agent said on the way out.
	StoppedBy string `json:"stoppedBy,omitempty"`
	// Team is who the task counts against, for the team limit.
	Team string `json:"team,omitempty"`
	// Seq is the order this task arrived in. Queued is a second, and two tasks
	// submitted in one second are two messages a person sent in order, so the
	// clock alone cannot say which of them goes first.
	Seq     int64 `json:"seq,omitempty"`
	Exit    int   `json:"exit"`
	Queued  int64 `json:"queuedAt"`
	Started int64 `json:"startedAt,omitempty"`
	Ended   int64 `json:"endedAt,omitempty"`
}

// Lane is what keeps two turns from running at once. A room's agent keeps one conversation and one state, so its own turns go
// one at a time while the room's other agents run beside it, each on a working copy of the room's folder. A turn in any other
// conversation shares its instance's one workspace, so the instance is its lane. A job that is not a turn has none.
func (r Record) Lane() string {
	switch {
	case r.Instance == "":
		return ""
	case r.Agent != "":
		return r.Instance + "/" + r.Agent
	}
	return r.Instance
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
