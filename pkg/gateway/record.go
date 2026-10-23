package gateway

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// RecordFile is the task's record of its tool calls, in the task's out folder, one JSON line each.
const RecordFile = "gateway.jsonl"

// Entry is one line of the record: a call, a refusal, or a listing of tools.
type Entry struct {
	At     int64  `json:"at"`
	Job    string `json:"job"`
	Person string `json:"person,omitempty"`
	Kind   string `json:"kind"`
	Server string `json:"server,omitempty"`
	Tool   string `json:"tool,omitempty"`
	// Outcome is "ok", "error" (the tool said it failed), "refused" (the policy said no) or "failed" (the server could not be
	// reached).
	Outcome string `json:"outcome,omitempty"`
	Why     string `json:"why,omitempty"`
	Millis  int64  `json:"ms,omitempty"`
	// ArgBytes and ResultBytes are how much went in and came back, not what it was.
	ArgBytes    int `json:"argBytes,omitempty"`
	ResultBytes int `json:"resultBytes,omitempty"`
	// Redacted is how many of each kind of data were masked in what the tool answered. What was masked is not kept.
	Redacted map[string]int `json:"redacted,omitempty"`
	// Definitions are how many tools there were and about how many tokens loading all their definitions would cost, against what
	// the task was shown.
	Tools       int `json:"tools,omitempty"`
	TokensAll   int `json:"tokensAll,omitempty"`
	TokensShown int `json:"tokensShown,omitempty"`
}

// Record appends to a task's record.
type Record struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
}

// OpenRecord is the record at a path, made when the first line is written.
func OpenRecord(path string) *Record { return &Record{path: path, now: time.Now} }

// Add writes a line. A record that cannot be written is said to the caller, and a call that cannot be recorded is not made.
func (r *Record) Add(entry Entry) error {
	if r == nil {
		return nil
	}
	entry.At = r.now().Unix()
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	file, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	return err
}
