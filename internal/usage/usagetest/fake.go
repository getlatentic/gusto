// Package usagetest is an in-memory usage.Store for tests of code that reads or writes the index.
package usagetest

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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

func (f *Fake) Add(ctx context.Context, rows []usage.Row) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if f.OnlySessions && !slices.Contains(f.Sessions, r.Session) {
		return false
	}
	if f.Search == "" {
		return true
	}
	needle := strings.ToLower(f.Search)
	for _, s := range []string{r.Session, r.Person, r.Agent, r.Engine, r.Provider, r.Model} {
		if strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
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

func (f *Fake) Totals(ctx context.Context, filter usage.Filter) (usage.Totals, error) {
	if err := ctx.Err(); err != nil {
		return usage.Totals{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var t usage.Totals
	for _, r := range f.matching(filter) {
		add(&t, r)
	}
	return t, nil
}

func (f *Fake) Groups(ctx context.Context, filter usage.Filter, by string) ([]usage.Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if by != usage.ByPerson && by != usage.ByEngine {
		return nil, usage.ErrBadGroup
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	index := map[string]*usage.Group{}
	agents := map[string]map[string]bool{}
	for _, r := range f.matching(filter) {
		value := r.Person
		if by == usage.ByEngine {
			value = r.Engine
		}
		if index[value] == nil {
			index[value], agents[value] = &usage.Group{Value: value}, map[string]bool{}
		}
		add(&index[value].Totals, r)
		if r.Agent != "" {
			agents[value][r.Agent] = true
		}
	}
	var out []usage.Group
	for value, g := range index {
		g.Agents = int64(len(agents[value]))
		out = append(out, *g)
	}
	usage.SortGroups(out)
	return out, nil
}

// before is the order calls were made in: by time, then by task and line.
func before(a, b usage.Row) bool {
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	if a.Task != b.Task {
		return a.Task < b.Task
	}
	return a.Line < b.Line
}

func (f *Fake) Sessions(ctx context.Context, filter usage.Filter, page usage.Page) ([]usage.Session, usage.Cursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	name, rank, err := sortOf(page.Sort)
	if err != nil {
		return nil, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.sessions(filter)
	sort.Slice(all, func(i, j int) bool {
		if ri, rj := rank(all[i]), rank(all[j]); ri != rj {
			return ri > rj
		}
		return all[i].ID < all[j].ID
	})
	start := 0
	if page.After != "" {
		after, id, err := parseSessionCursor(page.After, name)
		if err != nil {
			return nil, "", err
		}
		start = sort.Search(len(all), func(i int) bool {
			if r := rank(all[i]); r != after {
				return r < after
			}
			return all[i].ID > id
		})
	}
	end := min(start+limitOf(page), len(all))
	next := usage.Cursor("")
	if end < len(all) {
		next = sessionCursor(name, rank(all[end-1]), all[end-1].ID)
	}
	out := make([]usage.Session, 0, end-start)
	for _, s := range all[start:end] {
		out = append(out, s.Session)
	}
	return out, next, nil
}

// sortOf names a sort and ranks a session by it; the greater rank comes first.
func sortOf(name string) (string, func(fsession) int64, error) {
	switch name {
	case "", "last":
		return "last", func(s fsession) int64 { return s.Last.UnixNano() }, nil
	case "calls":
		return name, func(s fsession) int64 { return s.Calls }, nil
	case "tokens":
		return name, func(s fsession) int64 { return s.InputTokens + s.OutputTokens }, nil
	case "cost":
		return name, func(s fsession) int64 {
			if s.Calls == s.Unmeasured {
				return -1
			}
			return s.nanoUSD
		}, nil
	}
	return "", nil, fmt.Errorf("%w %q", usage.ErrBadSort, name)
}

func sessionCursor(sortName string, rank int64, id string) usage.Cursor {
	return usage.Cursor("sessions|" + sortName + "|" + strconv.FormatInt(rank, 10) + "|" + id)
}

func parseSessionCursor(c usage.Cursor, sortName string) (int64, string, error) {
	parts := strings.SplitN(string(c), "|", 4)
	if len(parts) != 4 || parts[0] != "sessions" || parts[1] != sortName {
		return 0, "", usage.ErrBadCursor
	}
	n, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, "", usage.ErrBadCursor
	}
	return n, parts[3], nil
}

// fsession is a session with the money it cost in billionths of a dollar, which is what "cost" ranks by.
type fsession struct {
	usage.Session
	nanoUSD int64
}

// sessions adds up the matching calls by session.
func (f *Fake) sessions(filter usage.Filter) []fsession {
	rows := f.matching(filter)
	sort.Slice(rows, func(i, j int) bool { return before(rows[i], rows[j]) })
	by := map[string]*fsession{}
	used := map[string]*firstUse{}
	for _, r := range rows {
		s := by[r.Session]
		if s == nil {
			s = &fsession{Session: usage.Session{ID: r.Session, Person: r.Person, Agent: r.Agent, Engine: r.Engine, First: r.At}}
			by[r.Session] = s
			used[r.Session] = newFirstUse()
		}
		s.Last = r.At
		used[r.Session].note(r)
		add(&s.Totals, r)
		if r.USD != nil {
			s.nanoUSD += int64(math.Round(*r.USD * 1e9))
		}
	}
	all := make([]fsession, 0, len(by))
	for id, s := range by {
		s.Providers, s.Models, s.Tasks = used[id].lists()
		all = append(all, *s)
	}
	return all
}

// firstUse remembers when each provider, model and task was first used, so that they list in that order.
type firstUse struct{ provider, model, task map[string]time.Time }

func newFirstUse() *firstUse {
	return &firstUse{provider: map[string]time.Time{}, model: map[string]time.Time{}, task: map[string]time.Time{}}
}

func (u *firstUse) note(r usage.Row) {
	first(u.provider, r.Provider, r.At)
	first(u.model, r.Model, r.At)
	first(u.task, r.Task, r.At)
}

func first(seen map[string]time.Time, value string, at time.Time) {
	if _, ok := seen[value]; !ok {
		seen[value] = at
	}
}

func (u *firstUse) lists() (providers, models, tasks []string) {
	return byFirstUse(u.provider), byFirstUse(u.model), byFirstUse(u.task)
}

func byFirstUse(first map[string]time.Time) []string {
	var out []string
	for v := range first {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if !first[out[i]].Equal(first[out[j]]) {
			return first[out[i]].Before(first[out[j]])
		}
		return out[i] < out[j]
	})
	return out
}

func limitOf(p usage.Page) int {
	if p.Limit <= 0 {
		return 50
	}
	return min(p.Limit, 1000)
}

func (f *Fake) Calls(ctx context.Context, session string, filter usage.Filter, page usage.Page) ([]usage.Row, usage.Cursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	held := false
	var rows []usage.Row
	for _, r := range f.rows {
		if r.Session == session {
			held = true
			if matches(r, filter) {
				rows = append(rows, r)
			}
		}
	}
	if !held {
		return nil, "", usage.ErrNotFound
	}
	sort.Slice(rows, func(i, j int) bool { return before(rows[j], rows[i]) })
	start := 0
	if page.After != "" {
		cut, err := parseCallCursor(page.After)
		if err != nil {
			return nil, "", err
		}
		start = sort.Search(len(rows), func(i int) bool { return before(rows[i], cut) })
	}
	end := min(start+limitOf(page), len(rows))
	next := usage.Cursor("")
	if end < len(rows) {
		last := rows[end-1]
		next = usage.Cursor("calls|" + strconv.FormatInt(last.At.UnixNano(), 10) + "|" + strconv.Itoa(last.Line) + "|" + last.Task)
	}
	return rows[start:end], next, nil
}

func parseCallCursor(c usage.Cursor) (usage.Row, error) {
	parts := strings.SplitN(string(c), "|", 4)
	if len(parts) != 4 || parts[0] != "calls" {
		return usage.Row{}, usage.ErrBadCursor
	}
	ns, errAt := strconv.ParseInt(parts[1], 10, 64)
	line, errLine := strconv.Atoi(parts[2])
	if errAt != nil || errLine != nil {
		return usage.Row{}, usage.ErrBadCursor
	}
	return usage.Row{At: time.Unix(0, ns), Task: parts[3], Line: line}, nil
}

func (f *Fake) Export(ctx context.Context, filter usage.Filter, fn func(usage.Row) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	rows := f.matching(filter)
	f.mu.Unlock()
	sort.Slice(rows, func(i, j int) bool { return before(rows[j], rows[i]) })
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fake) Reset(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int64(len(f.rows))
	f.rows = map[string]usage.Row{}
	return n, nil
}

func (f *Fake) Facets(ctx context.Context, filter usage.Filter) (usage.Facets, error) {
	if err := ctx.Err(); err != nil {
		return usage.Facets{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out usage.Facets
	for _, r := range f.matching(filter) {
		for _, c := range []struct {
			into  *[]string
			value string
		}{{&out.People, r.Person}, {&out.Teams, r.Team}, {&out.Projects, r.Project}, {&out.Engines, r.Engine},
			{&out.Agents, r.Agent}, {&out.Providers, r.Provider}, {&out.Models, r.Model}} {
			if c.value != "" {
				*c.into = appendOnce(*c.into, c.value)
			}
		}
	}
	for _, l := range [][]string{out.People, out.Teams, out.Projects, out.Engines, out.Agents, out.Providers, out.Models} {
		sort.Strings(l)
	}
	return out, nil
}

func appendOnce(list []string, s string) []string {
	for _, have := range list {
		if have == s {
			return list
		}
	}
	return append(list, s)
}

func (f *Fake) Indexed(ctx context.Context) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
