package web

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/internal/usage"
)

// failedListed is the most failed conversations the Failed view names to the index, the latest, so that the query naming them
// stays well inside what SQLite and Postgres take as arguments.
const failedListed = 1000

// usageState is one view of the table: every conversation, or the failed ones alone.
type usageState struct {
	Label, Href, Count string
	Current            bool
}

// usageStates are the views of the table, each keeping the filter. Failed says how many conversations failed this week.
func (s *Server) usageStates(query usageQuery, base string) []usageState {
	chosen := query.Values.Get("state")
	failed := failedAfter(s.failedConversations(query.Values.Get("person")), time.Now().Add(-lately))
	return []usageState{
		{Label: "All", Href: query.link(base, map[string]string{"state": "", "after": ""}), Current: chosen == ""},
		{Label: "Failed", Href: query.link(base, map[string]string{"state": stateFailed, "after": ""}), Count: strconv.Itoa(failed) + " this week",
			Current: chosen == stateFailed},
	}
}

// failedConversations are the latest turns of the conversations whose latest turn failed or was interrupted, the latest first,
// of the conversations a person pays for alone when one is named. Failed is what the record of tasks calls failed.
func (s *Server) failedConversations(person string) []tasks.Record {
	var failed []tasks.Record
	for _, record := range s.lastTurns(person) {
		if filed(record.State) == string(tasks.Failed) {
			failed = append(failed, record)
		}
	}
	slices.SortFunc(failed, func(a, b tasks.Record) int {
		switch {
		case later(a, b):
			return -1
		case later(b, a):
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return failed
}

// lastTurns is each conversation's latest task, by the conversation the usage index files its calls under.
func (s *Server) lastTurns(person string) map[string]tasks.Record {
	last := map[string]tasks.Record{}
	for _, record := range s.index.List() {
		if person != "" && !strings.EqualFold(record.Payer(), person) {
			continue
		}
		session := usage.SessionOf(record)
		if held, found := last[session]; !found || later(record, held) {
			last[session] = record
		}
	}
	return last
}

// later says a was asked for after b. Two tasks asked for in one second are in the order they arrived.
func later(a, b tasks.Record) bool {
	if a.Queued != b.Queued {
		return a.Queued > b.Queued
	}
	return a.Seq > b.Seq
}

// failedSessions names the latest failed conversations to the index.
func failedSessions(failed []tasks.Record) []string {
	ids := make([]string, 0, min(len(failed), failedListed))
	for _, record := range failed[:min(len(failed), failedListed)] {
		ids = append(ids, usage.SessionOf(record))
	}
	return ids
}

// failedAfter is how many of the failed conversations last failed on a turn asked for after a time.
func failedAfter(failed []tasks.Record, since time.Time) int {
	n := 0
	for _, record := range failed {
		if record.Queued >= since.Unix() {
			n++
		}
	}
	return n
}
