package web

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
)

// taskQuery is what a request asks of the task index: the filter, and which page in which order. Values is the
// filter as it is written in a link, so that the next page keeps it.
type taskQuery struct {
	Filter taskindex.Filter
	Page   taskindex.Page
	Values url.Values
	// State is the state the request named, as written.
	State string
	// None is a request that may read no task at all.
	None bool
}

var taskStates = []string{"queued", "running", "passed", "failed", "interrupted"}

// taskFields are the query parameters that filter the list.
var taskFields = []string{"q", "state", "person", "agent_type", "team", "repo", "instance", "thread", "from", "to"}

// taskScope is whose tasks the request may read: nobody's name for all of them, and otherwise the one person. It
// is the rule ownJobs applies to a list in memory, so that the same person sees the same tasks either way.
func (s *Server) taskScope(r *http.Request) (asked string, none bool) {
	if s.admin(r) {
		return "", false
	}
	name := s.askerName(r)
	return name, name == "" || (name == sharedSignIn && !s.soleOwner())
}

// readTaskQuery reads the filter from the query and forces its person to the one the request may read. forPage
// makes the state failed stand for the tasks that did not pass, as the list page counts them, and a state that
// is none of the five match nothing, where the API refuses it.
func (s *Server) readTaskQuery(r *http.Request, forPage bool) (taskQuery, error) {
	asked := r.URL.Query()
	made := taskQuery{Values: url.Values{}}
	for _, name := range taskFields {
		if value := strings.TrimSpace(asked.Get(name)); value != "" {
			made.Values.Set(name, value)
		}
	}
	if value := strings.TrimSpace(asked.Get("harness")); value != "" && made.Values.Get("agent_type") == "" {
		made.Values.Set("agent_type", value)
	}
	person, none := s.taskScope(r)
	made.None = none
	if person != "" {
		made.Values.Set("person", person)
	}
	f := &made.Filter
	f.Search, f.Asked, f.Team, f.Repo = made.Values.Get("q"), made.Values.Get("person"), made.Values.Get("team"), made.Values.Get("repo")
	f.AgentType, f.Instance, f.Thread = made.Values.Get("agent_type"), made.Values.Get("instance"), made.Values.Get("thread")
	made.State = made.Values.Get("state")
	var err error
	if f.States, err = taskStatesOf(made.State, forPage); err != nil {
		return made, err
	}
	if f.QueuedFrom, err = usageTime(made.Values.Get("from"), false); err != nil {
		return made, fmt.Errorf("from: %w", err)
	}
	if f.QueuedTo, err = usageTime(made.Values.Get("to"), true); err != nil {
		return made, fmt.Errorf("to: %w", err)
	}
	if made.Page.Limit, err = taskLimit(asked.Get("limit")); err != nil {
		return made, err
	}
	made.Page.After = taskindex.Cursor(asked.Get("after"))
	if _, err = taskindex.SortFor(asked.Get("sort")); err != nil {
		return made, err
	}
	made.Page.Sort = asked.Get("sort")
	return made, nil
}

// taskStatesOf reads a state, or a comma-separated list of them.
func taskStatesOf(text string, forPage bool) ([]string, error) {
	if text == "" {
		return nil, nil
	}
	var states []string
	for _, state := range strings.Split(text, ",") {
		state = strings.TrimSpace(state)
		if !slices.Contains(taskStates, state) && !forPage {
			return nil, fmt.Errorf("state must be one of %s, not %q", strings.Join(taskStates, ", "), state)
		}
		states = append(states, state)
		if forPage && state == "failed" {
			states = append(states, "interrupted")
		}
	}
	return states, nil
}

func taskLimit(text string) (int, error) {
	if text == "" {
		return taskindex.DefaultPerPage, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > taskindex.MostPerPage {
		return 0, fmt.Errorf("limit must be a number from 1 to %d", taskindex.MostPerPage)
	}
	return n, nil
}

// link is this query as an address under base, with the changes given: an empty value takes a name out.
func (q taskQuery) link(base string, change map[string]string) string {
	values := url.Values{}
	for name, held := range q.Values {
		values[name] = held
	}
	if q.Page.Sort != "" {
		values.Set("sort", q.Page.Sort)
	}
	if q.Page.Limit != taskindex.DefaultPerPage {
		values.Set("limit", strconv.Itoa(q.Page.Limit))
	}
	for name, value := range change {
		if value == "" {
			values.Del(name)
		} else {
			values.Set(name, value)
		}
	}
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}

// sinceLately is the start of the week the counts across the top of the list look at.
func sinceLately(now time.Time) time.Time { return now.Add(-lately) }
