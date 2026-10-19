// Package usage keeps one row for each model call, so an organisation can search,
// filter and add up what its agents spent: by session, person, agent, engine,
// provider and model.
//
// The task's out/proxy.jsonl stays the record, and is what the manifest hashes. These
// rows are an index built from it: one that is deleted is rebuilt from the files, and
// a rebuilt index holds the same rows. A call has the key (Task, Line), where Line is
// its place in the task's proxy.jsonl, so adding a row twice is adding it once.
package usage

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is a session or call that the index does not hold.
var ErrNotFound = errors.New("usage: not found")

// Row is one model call, with who made it.
type Row struct {
	Task string    `json:"task"`
	Line int       `json:"line"`
	At   time.Time `json:"at"`
	// Session is the conversation a call belongs to: the instance and thread when the task is a turn
	// in one, and the task itself otherwise.
	Session string `json:"session"`
	// Person is who asked. A call made with a script's token has that token's name.
	Person  string `json:"person"`
	Team    string `json:"team,omitempty"`
	Project string `json:"project,omitempty"`
	// Engine is the program that made the call: Claude Code, Codex and the others. Agent is the name a person
	// gave the agent they set up, and is empty when the task was not made through one.
	Engine string `json:"engine"`
	Agent  string `json:"agent,omitempty"`
	// Provider is who answered, by the name the organisation's model registry gives it, and Credential is
	// the name of the key the proxy added. The key itself is never here.
	Provider   string `json:"provider"`
	Credential string `json:"credential,omitempty"`
	Model      string `json:"model"`
	// The counts and the cost are nil when the answer carried no usage or the model has no price, so that
	// a missing number is not read as a free call.
	InputTokens  *int64   `json:"inputTokens"`
	OutputTokens *int64   `json:"outputTokens"`
	CachedTokens *int64   `json:"cachedTokens,omitempty"`
	USD          *float64 `json:"usd"`
	Status       int      `json:"status,omitempty"`
	TookMS       int64    `json:"tookMs"`
	// Refused is the cap that kept the call from leaving, and is empty for a call that went.
	Refused    string `json:"refused,omitempty"`
	Translated string `json:"translated,omitempty"`
}

// Filter narrows rows. A zero field matches everything. Search matches part of a session, person,
// agent, engine, provider or model, without regard to case.
type Filter struct {
	From, To time.Time
	Person   string
	Team     string
	Project  string
	Engine   string
	Agent    string
	Provider string
	Model    string
	Search   string
}

// Cursor continues a listing where the last page ended. It is opaque, and empty at the start and at the end. It
// is valid only for the method, and for Sessions the sort, that made it; any other use is ErrBadCursor.
type Cursor string

// Page is how many rows to return, and where to start. A Limit of zero is 50, and no page holds more than 1000.
// Sort ranks sessions, always from the greatest: "last" (the default) is the latest call, "cost" the sum of
// the measured USD (a session with no measured call comes last), "calls" the number of calls, and "tokens" the
// input and output tokens together. Sessions of equal rank come in the order of their ids. Calls ignores Sort,
// and an unknown one is ErrBadSort.
type Page struct {
	Limit int
	After Cursor
	Sort  string
}

// Session is the calls of one conversation, added up. Providers and Models are in the order of first use.
type Session struct {
	ID        string    `json:"id"`
	Person    string    `json:"person"`
	Agent     string    `json:"agent,omitempty"`
	Engine    string    `json:"engine"`
	Providers []string  `json:"providers"`
	Models    []string  `json:"models"`
	Tasks     []string  `json:"tasks"`
	First     time.Time `json:"first"`
	Last      time.Time `json:"last"`
	Totals
}

// Totals add up calls. USD counts only measured calls, and Unmeasured counts the rest, so a total
// is never presented as complete when it is not.
type Totals struct {
	Calls        int64   `json:"calls"`
	Unmeasured   int64   `json:"unmeasured"`
	Refused      int64   `json:"refused"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CachedTokens int64   `json:"cachedTokens"`
	USD          float64 `json:"usd"`
}

// Facets are the values a filter may be given, for the selects on a page.
type Facets struct {
	People    []string `json:"people"`
	Teams     []string `json:"teams"`
	Projects  []string `json:"projects"`
	Engines   []string `json:"engines"`
	Agents    []string `json:"agents"`
	Providers []string `json:"providers"`
	Models    []string `json:"models"`
}

// Store is the index. Every method is safe to call from many goroutines, and a SQLite file may be used by more
// than one process at once (it keeps a write-ahead log and waits for a busy lock). A list that has no value, such
// as the Facets of an empty index, is nil.
type Store interface {
	// Add inserts rows. A row whose (Task, Line) is held is left as it is, so a rebuild may add all of a
	// task's calls again.
	Add(ctx context.Context, rows []Row) error
	// Sessions lists sessions that have a call matching the filter, in the order the page's Sort gives.
	// Each session's totals count only the calls that match.
	Sessions(ctx context.Context, filter Filter, page Page) ([]Session, Cursor, error)
	// Calls lists the calls of one session that match the filter, the latest first. A session none of whose
	// calls match gives an empty page; ErrNotFound is for a session the index holds no call of at all.
	Calls(ctx context.Context, session string, filter Filter, page Page) ([]Row, Cursor, error)
	// Export calls fn with every call that matches the filter, the latest first, in batches so that memory
	// stays flat however many there are. It stops at the first error from fn, and returns it.
	Export(ctx context.Context, filter Filter, fn func(Row) error) error
	Totals(ctx context.Context, filter Filter) (Totals, error)
	// Facets lists the distinct values, in order, of the calls that match the filter.
	Facets(ctx context.Context, filter Filter) (Facets, error)
	// Indexed is, for each task the index holds, how many of its calls it holds: one more than the highest Line.
	// A rebuild reads only the lines past it.
	Indexed(ctx context.Context) (map[string]int, error)
	// Reset deletes every row and says how many it removed, which is how an index is rebuilt from scratch.
	Reset(ctx context.Context) (int64, error)
	Close() error
}

// Who says who made the calls of a task. It is what the proxy cannot know for itself,
// and what the server, which holds the task's record and its conversation, can.
type Who struct {
	Session string
	Person  string
	Team    string
	Project string
	Engine  string
	Agent   string
}

// WhoFunc finds who made a task's calls, and is false for a task it does not know.
type WhoFunc func(task string) (Who, bool)

// ProviderFunc names who answers a model, by the name the organisation's model registry gives it.
type ProviderFunc func(model, credential string) string

// Status is how far the index has caught up with the files. A page shows it while Building is true,
// so that an index still being filled is not read as an empty month.
type Status struct {
	Building bool `json:"building"`
	Done     int  `json:"done"`
	Total    int  `json:"total"`
}
