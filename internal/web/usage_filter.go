package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/usage"
)

const (
	usageDefaultLimit = 50
	usageMostPerPage  = 1000
)

// usageQuery is what a request asks of the index: the filter, and which page, in which order (Page.Sort).
// Values is the filter as it is written in a link, so that the next page and the export keep it.
type usageQuery struct {
	Filter usage.Filter
	Page   usage.Page
	Values url.Values
}

type usageEveryoneKey struct{}

// seesEveryone marks a request made with an organisation's token, or the shared password, which is not about one person.
func seesEveryone(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), usageEveryoneKey{}, true))
}

// usageScope is whose calls the request may read: nobody's name for all of them, and otherwise the one person.
// It is false for a request that is neither an admin's nor about a person.
func (s *Server) usageScope(r *http.Request) (person string, allowed bool) {
	if everyone, _ := r.Context().Value(usageEveryoneKey{}).(bool); everyone || s.admin(r) {
		return "", true
	}
	person = s.askerName(r)
	return person, person != ""
}

// usageAccess lets in what the other calls of the API let in: a person's token, which reads that person's calls, an
// organisation's token with the spend scope, which reads all of them, and a person signed in at the pages.
func (s *Server) usageAccess(next http.HandlerFunc) http.HandlerFunc {
	forPerson := s.sent(apitoken.Work, next)
	forOrganisation := s.sent(apitoken.Spend, func(w http.ResponseWriter, r *http.Request) { next(w, seesEveryone(r)) })
	return func(w http.ResponseWriter, r *http.Request) {
		bearer, bearing := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case bearing && s.holdsPersonToken(bearer):
			forPerson(w, r)
		case !bearing && s.keeper.knows(r):
			next(w, r)
		default:
			forOrganisation(w, r)
		}
	}
}

func (s *Server) holdsPersonToken(secret string) bool {
	if s.tokens == nil {
		return false
	}
	token, err := s.tokens.Check(secret)
	return err == nil && token.Person != "" && token.May(apitoken.Work)
}

var usageFields = []string{"q", "person", "team", "project", "agent_type", "agent", "provider", "model", "from", "to"}

// readUsageQuery reads the filter from the query, and forces its person to the one the request may read.
func (s *Server) readUsageQuery(r *http.Request) (usageQuery, bool, error) {
	person, allowed := s.usageScope(r)
	if !allowed {
		return usageQuery{}, false, nil
	}
	asked := r.URL.Query()
	made := usageQuery{Values: url.Values{}}
	for _, name := range usageFields {
		if value := strings.TrimSpace(asked.Get(name)); value != "" {
			made.Values.Set(name, value)
		}
	}
	if person != "" {
		made.Values.Set("person", person)
	}
	f := usage.Filter{Search: made.Values.Get("q"), Person: made.Values.Get("person"), Team: made.Values.Get("team"),
		Project: made.Values.Get("project"), Engine: made.Values.Get("agent_type"), Agent: made.Values.Get("agent"),
		Provider: made.Values.Get("provider"), Model: made.Values.Get("model")}
	var err error
	if f.From, err = usageTime(made.Values.Get("from"), false); err != nil {
		return made, true, fmt.Errorf("from: %w", err)
	}
	if f.To, err = usageTime(made.Values.Get("to"), true); err != nil {
		return made, true, fmt.Errorf("to: %w", err)
	}
	made.Filter = f
	if made.Page.Limit, err = usageLimit(asked.Get("limit")); err != nil {
		return made, true, err
	}
	made.Page.After = usage.Cursor(asked.Get("after"))
	if made.Page.Sort, err = usageSort(asked.Get("sort")); err != nil {
		return made, true, err
	}
	return made, true, nil
}

// usageTime reads an RFC 3339 time, or a date, which is the start of that day in UTC and, for the end of a range, the
// start of the next, so that a range ending on a date includes the date.
func usageTime(text string, end bool) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	if at, err := time.Parse(time.RFC3339, text); err == nil {
		return at, nil
	}
	day, err := time.Parse("2006-01-02", text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither an RFC 3339 time nor a date like 2026-10-19", text)
	}
	if end {
		day = day.AddDate(0, 0, 1)
	}
	return day, nil
}

// usageSort is the order the sessions come in, from the greatest. Empty is the index's own, the latest use.
func usageSort(text string) (string, error) {
	switch text {
	case "", "last", "cost", "calls", "tokens":
		return text, nil
	}
	return "", fmt.Errorf("sort must be one of last, cost, calls or tokens, not %q", text)
}

func usageLimit(text string) (int, error) {
	if text == "" {
		return usageDefaultLimit, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > usageMostPerPage {
		return 0, fmt.Errorf("limit must be a number from 1 to %d", usageMostPerPage)
	}
	return n, nil
}

// link is this query as an address under base, with the changes given: an empty value takes a name out.
func (q usageQuery) link(base string, change map[string]string) string {
	values := url.Values{}
	for name, held := range q.Values {
		values[name] = held
	}
	if q.Page.Sort != "" {
		values.Set("sort", q.Page.Sort)
	}
	if q.Page.Limit != usageDefaultLimit {
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
