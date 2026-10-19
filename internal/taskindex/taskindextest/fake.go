// Package taskindextest is an in-memory taskindex.Store for tests of code that reads or writes the index.
package taskindextest

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/getlatentic/gusto/internal/taskindex"
)

// Fake holds rows in memory. It follows the same rules as the real stores, and is what their shared contract
// test is first run against.
type Fake struct {
	mu   sync.Mutex
	rows map[string]taskindex.Row
}

var _ taskindex.Store = (*Fake)(nil)

func New() *Fake { return &Fake{rows: map[string]taskindex.Row{}} }

func (f *Fake) Close() error { return nil }

func (f *Fake) Upsert(ctx context.Context, row taskindex.Row) error {
	return f.UpsertMany(ctx, []taskindex.Row{row})
}

func (f *Fake) UpsertMany(ctx context.Context, rows []taskindex.Row) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rows {
		f.rows[r.ID] = r
	}
	return nil
}

func (f *Fake) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
}

func (f *Fake) Get(ctx context.Context, id string) (taskindex.Row, error) {
	if err := ctx.Err(); err != nil {
		return taskindex.Row{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, held := f.rows[id]; held {
		return r, nil
	}
	return taskindex.Row{}, taskindex.ErrNotFound
}

func (f *Fake) FindKey(ctx context.Context, asked, key string) (taskindex.Row, error) {
	if err := ctx.Err(); err != nil {
		return taskindex.Row{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if key != "" && r.RequestKey == key && r.Asked == asked {
			return r, nil
		}
	}
	return taskindex.Row{}, taskindex.ErrNotFound
}

func matches(r taskindex.Row, f taskindex.Filter) bool {
	if len(f.States) > 0 && !slices.Contains(f.States, r.State) {
		return false
	}
	if !f.QueuedFrom.IsZero() && r.Queued < f.QueuedFrom.Unix() || !f.QueuedTo.IsZero() && r.Queued >= f.QueuedTo.Unix() {
		return false
	}
	if f.Asked != "" && strings.ToLower(f.Asked) != strings.ToLower(r.Asked) {
		return false
	}
	for _, c := range [][2]string{{f.Team, r.Team}, {f.Repo, r.Repo}, {f.AgentType, r.AgentType}, {f.Instance, r.Instance}, {f.Thread, r.Thread}} {
		if c[0] != "" && c[0] != c[1] {
			return false
		}
	}
	return f.Search == "" || r.Matches(f.Search)
}

// ordered is the rows that match the filter in the order of the sort.
func (f *Fake) ordered(filter taskindex.Filter, by taskindex.Sort) []taskindex.Row {
	var out []taskindex.Row
	for _, r := range f.rows {
		if matches(r, filter) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return by.PositionOf(out[j]).After(by.PositionOf(out[i])) })
	return out
}

func (f *Fake) List(ctx context.Context, filter taskindex.Filter, page taskindex.Page) ([]taskindex.Row, taskindex.Cursor, error) {
	by, err := taskindex.SortFor(page.Sort)
	if err != nil {
		return nil, "", err
	}
	var after *taskindex.Position
	if page.After != "" {
		p, err := by.ParseCursor(page.After)
		if err != nil {
			return nil, "", err
		}
		after = &p
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	limit := taskindex.PageSize(page)
	var out []taskindex.Row
	for _, r := range f.ordered(filter, by) {
		if after != nil && !by.PositionOf(r).After(*after) {
			continue
		}
		if out = append(out, r); len(out) > limit {
			return out[:limit], by.NewCursor(by.PositionOf(out[limit-1])), nil
		}
	}
	return out, "", nil
}

func (f *Fake) Count(ctx context.Context, filter taskindex.Filter) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := map[string]int{}
	for _, r := range f.rows {
		if matches(r, filter) {
			counts[r.State]++
		}
	}
	return counts, nil
}

func (f *Fake) AgentTypes(ctx context.Context, filter taskindex.Filter) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.rows {
		if matches(r, filter) && !slices.Contains(out, r.AgentType) {
			out = append(out, r.AgentType)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (f *Fake) Indexed(ctx context.Context) (map[string]taskindex.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]taskindex.Entry, len(f.rows))
	for id, r := range f.rows {
		out[id] = r.Entry()
	}
	return out, nil
}

func (f *Fake) Reset(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int64(len(f.rows))
	clear(f.rows)
	return n, nil
}

// All is every row, in no order, for a test to look at.
func (f *Fake) All() []taskindex.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]taskindex.Row, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out
}
