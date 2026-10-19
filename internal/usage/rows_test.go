package usage_test

import (
	"fmt"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
)

var epoch = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func p64(v int64) *int64    { return &v }
func pf(v float64) *float64 { return &v }

// at is a moment, in seconds after the epoch, kept to whole microseconds as Postgres keeps them.
func at(sec int) time.Time { return epoch.Add(time.Duration(sec) * time.Second) }

// callRow is a measured call of session s at second sec, made by ada with Claude Code and Anthropic.
func callRow(task string, line int, session string, sec int) usage.Row {
	return usage.Row{
		Task: task, Line: line, At: at(sec), Session: session, Person: "ada", Engine: "claude-code",
		Provider: "anthropic", Model: "sonnet", InputTokens: p64(10), OutputTokens: p64(5),
		USD: pf(0.5), Status: 200, TookMS: 120,
	}
}

func sessionIDs(list []usage.Session) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.ID
	}
	return out
}

func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%04d", prefix, i)
	}
	return out
}
