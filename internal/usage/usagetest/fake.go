// Package usagetest is an in-memory usage.Store for tests of code that reads or writes the index.
package usagetest

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/getlatentic/gusto/internal/usage"
)

// Fake holds rows in memory. It follows the same rules as the real stores, and
// is what their shared contract test is first run against.
type Fake struct {
	mu   sync.Mutex
	rows map[string]usage.Row
}

func New() *Fake { return &Fake{rows: map[string]usage.Row{}} }

func key(r usage.Row) string { return r.Task + "/" + strconv.Itoa(r.Line) }

func (f *Fake) Add(_ context.Context, rows []usage.Row) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rows {
		if _, held := f.rows[key(r)]; !held {
			f.rows[key(r)] = r
		}
	}
	return nil
}

// All is every row, in no order, for a test to look at.
func (f *Fake) All() []usage.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]usage.Row, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out
}

func matches(r usage.Row, f usage.Filter) bool {
	if !f.From.IsZero() && r.At.Before(f.From) || !f.To.IsZero() && !r.At.Before(f.To) {
		return false
	}
	for _, c := range [][2]string{{f.Person, r.Person}, {f.Team, r.Team}, {f.Project, r.Project}, {f.Engine, r.Engine}, {f.Agent, r.Agent}, {f.Provider, r.Provider}, {f.Model, r.Model}} {
		if c[0] != "" && c[0] != c[1] {
			return false
		}
	}
	if f.Search != "" {
		needle := strings.ToLower(f.Search)
		hit := false
		for _, s := range []string{r.Session, r.Person, r.Agent, r.Engine, r.Provider, r.Model} {
			if strings.Contains(strings.ToLower(s), needle) {
				hit = true
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func (f *Fake) matching(filter usage.Filter) []usage.Row {
	var out []usage.Row
	for _, r := range f.rows {
		if matches(r, filter) {
			out = append(out, r)
		}
	}
	return out
}

func add(t *usage.Totals, r usage.Row) {
	t.Calls++
	if r.Refused != "" {
		t.Refused++
	}
	if r.USD == nil {
		t.Unmeasured++
	} else {
		t.USD += *r.USD
	}
	if r.InputTokens != nil {
		t.InputTokens += *r.InputTokens
	}
	if r.OutputTokens != nil {
		t.OutputTokens += *r.OutputTokens
	}
	if r.CachedTokens != nil {
		t.CachedTokens += *r.CachedTokens
	}
}

func (f *Fake) Totals(_ context.Context, filter usage.Filter) (usage.Totals, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var t usage.Totals
	for _, r := range f.matching(filter) {
		add(&t, r)
	}
	return t, nil
}

func (f *Fake) Sessions(_ context.Context, filter usage.Filter, page usage.Page) ([]usage.Session, usage.Cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	by := map[string]*usage.Session{}
	rows := f.matching(filter)
	sort.Slice(rows, func(i, j int) bool { return rows[i].At.Before(rows[j].At) })
	for _, r := range rows {
		s := by[r.Session]
		if s == nil {
			s = &usage.Session{ID: r.Session, Person: r.Person, Agent: r.Agent, Engine: r.Engine, First: r.At}
			by[r.Session] = s
		}
		s.Last = r.At
		s.Providers = appendOnce(s.Providers, r.Provider)
		s.Models = appendOnce(s.Models, r.Model)
		s.Tasks = appendOnce(s.Tasks, r.Task)
		add(&s.Totals, r)
	}
	all := make([]usage.Session, 0, len(by))
	for _, s := range by {
		all = append(all, *s)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].Last.Equal(all[j].Last) {
			return all[i].Last.After(all[j].Last)
		}
		return all[i].ID < all[j].ID
	})
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	start := 0
	if page.After != "" {
		for i, s := range all {
			if s.ID == string(page.After) {
				start = i + 1
			}
		}
	}
	end := start + limit
	next := usage.Cursor("")
	if end < len(all) {
		next = usage.Cursor(all[end-1].ID)
	} else {
		end = len(all)
	}
	return all[start:end], next, nil
}

func appendOnce(list []string, s string) []string {
	for _, have := range list {
		if have == s {
			return list
		}
	}
	return append(list, s)
}

func (f *Fake) Calls(_ context.Context, session string, page usage.Page) ([]usage.Row, usage.Cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var rows []usage.Row
	for _, r := range f.rows {
		if r.Session == session {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return nil, "", usage.ErrNotFound
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].At.Equal(rows[j].At) {
			return rows[i].At.After(rows[j].At)
		}
		return rows[i].Task+strconv.Itoa(rows[i].Line) > rows[j].Task+strconv.Itoa(rows[j].Line)
	})
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	start := 0
	if page.After != "" {
		for i, r := range rows {
			if r.Task+"/"+strconv.Itoa(r.Line) == string(page.After) {
				start = i + 1
			}
		}
	}
	end := start + limit
	next := usage.Cursor("")
	if end < len(rows) {
		next = usage.Cursor(rows[end-1].Task + "/" + strconv.Itoa(rows[end-1].Line))
	} else {
		end = len(rows)
	}
	return rows[start:end], next, nil
}

func (f *Fake) Facets(_ context.Context, filter usage.Filter) (usage.Facets, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out usage.Facets
	for _, r := range f.matching(filter) {
		out.People = appendOnce(out.People, r.Person)
		if r.Team != "" {
			out.Teams = appendOnce(out.Teams, r.Team)
		}
		if r.Project != "" {
			out.Projects = appendOnce(out.Projects, r.Project)
		}
		out.Engines = appendOnce(out.Engines, r.Engine)
		if r.Agent != "" {
			out.Agents = appendOnce(out.Agents, r.Agent)
		}
		out.Providers = appendOnce(out.Providers, r.Provider)
		out.Models = appendOnce(out.Models, r.Model)
	}
	for _, l := range [][]string{out.People, out.Teams, out.Projects, out.Engines, out.Agents, out.Providers, out.Models} {
		sort.Strings(l)
	}
	return out, nil
}

func (f *Fake) Indexed(_ context.Context) (map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for _, r := range f.rows {
		if r.Line+1 > out[r.Task] {
			out[r.Task] = r.Line + 1
		}
	}
	return out, nil
}

func (f *Fake) Close() error { return nil }
