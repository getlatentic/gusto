// Package taskindex keeps one row for each task, so an organization can list, search and page
// through its tasks without reading every task directory or holding every task in memory, and so that a
// second replica can read the same list.
//
// The task's state.json stays the truth. These rows are an index kept current as tasks change and rebuilt
// from the files when it drifts: an index that is deleted is rebuilt, and a rebuilt index holds the same rows.
package taskindex

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is a task the index does not hold.
var ErrNotFound = errors.New("taskindex: not found")

// Row is one task as the index holds it. It has the fields of the task's record that a list shows, filters
// by or orders by. State is the task's state as a word: queued, running, passed, failed or interrupted.
type Row struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Brief string `json:"brief"`
	Repo  string `json:"repo,omitempty"`
	// AgentType is the program that runs the task, and is kept in the column named harness.
	AgentType string `json:"agentType"`
	Model     string `json:"model,omitempty"`
	// Instance and Thread are set when the task is a turn in a conversation.
	Instance string `json:"instance,omitempty"`
	Thread   string `json:"thread,omitempty"`
	Asked    string `json:"asked"`
	// RequestKey is the id the asker gave the request, so that a send made twice is one task.
	RequestKey string `json:"requestId,omitempty"`
	StoppedBy  string `json:"stoppedBy,omitempty"`
	Team       string `json:"team,omitempty"`
	// Seq is the order the task arrived in. Times are in Unix seconds.
	Seq     int64 `json:"seq"`
	Exit    int   `json:"exit"`
	Queued  int64 `json:"queuedAt"`
	Started int64 `json:"startedAt,omitempty"`
	Ended   int64 `json:"endedAt,omitempty"`
}

// Filter narrows rows. A zero field matches everything. Asked and Search ignore case; the other text fields
// are exact. Search matches part of a task's id, brief, asker or repository, and takes % and _ as the
// characters they are.
type Filter struct {
	States    []string
	Asked     string
	Team      string
	Repo      string
	AgentType string
	Instance  string
	Thread    string
	// QueuedFrom is included and QueuedTo is not.
	QueuedFrom, QueuedTo time.Time
	Search               string
}

// Cursor continues a listing where the last page ended. It is opaque, and empty at the start and at the end.
// It is valid only for the sort that made it; any other use is ErrBadCursor.
type Cursor string

// Page is how many rows to return, where to start, and in which order. A Limit of zero is 50, and no page
// holds more than 1000. Sort is "queued" (the default), "ended" or "seq", always from the greatest: the
// task asked for last, ended last, or arrived last. Rows of equal rank come in descending order of their
// arrival and then of their ids. An unknown sort is ErrBadSort.
type Page struct {
	Limit int
	After Cursor
	Sort  string
}

// Entry is what the index holds of a task that a rebuild compares with the task's own record. Sum is a
// digest of the whole row, so a row that differs in any field is found.
type Entry struct {
	State string
	Ended int64
	Seq   int64
	Sum   string
}

// Store is the index. Every method is safe to call from many goroutines, and a SQLite file may be used by more
// than one process at once. A list that has no value is nil.
type Store interface {
	// Upsert inserts a task's row or replaces the one it has.
	Upsert(ctx context.Context, row Row) error
	// UpsertMany does Upsert for each row in one transaction at a time. A task named twice keeps its last row.
	UpsertMany(ctx context.Context, rows []Row) error
	// Delete removes a task's row. A task the index does not hold is not an error.
	Delete(ctx context.Context, id string) error
	// Get is a task's row, or ErrNotFound.
	Get(ctx context.Context, id string) (Row, error)
	// FindKey is the task the asker already made with this request id, or ErrNotFound. An id is the
	// asker's own: the same words from somebody else are another request.
	FindKey(ctx context.Context, asked, key string) (Row, error)
	// List returns the rows that match the filter in the order the page's Sort gives, and the cursor of the
	// next page, which is empty on the last.
	List(ctx context.Context, filter Filter, page Page) ([]Row, Cursor, error)
	// Count is how many rows match the filter, for each state that has any.
	Count(ctx context.Context, filter Filter) (map[string]int, error)
	// AgentTypes lists, in order, the agent types of the rows that match the filter.
	AgentTypes(ctx context.Context, filter Filter) ([]string, error)
	// Indexed is, for every task the index holds, what a rebuild compares with the task's record.
	Indexed(ctx context.Context) (map[string]Entry, error)
	// Reset deletes every row and says how many there were.
	Reset(ctx context.Context) (int64, error)
	Close() error
}
