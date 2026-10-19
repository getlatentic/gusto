package web

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
)

var usageCSVHeader = []string{"time", "session", "task", "line", "person", "team", "project", "agent", "agent_type", "provider", "key", "model",
	"input_tokens", "output_tokens", "cached_tokens", "cost_usd", "status", "took_ms", "refused", "translated"}

// usageCSV streams every call that matches the filter, the latest first, as the index gives them.
func (s *Server) usageCSV(w http.ResponseWriter, r *http.Request) {
	query, ok := s.usageReader(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="usage.csv"`)
	out := csv.NewWriter(w)
	_ = out.Write(usageCSVHeader)
	sent := false
	emit := func(row usage.Row) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if err := out.Write(csvLine(row)); err != nil {
			return err
		}
		sent = true
		out.Flush()
		return out.Error()
	}
	err := s.usage.store.Export(r.Context(), query.Filter, emit)
	switch {
	case err == nil:
		out.Flush()
	case r.Context().Err() != nil:
	case sent:
		// The status went with the first row, so a failure now can only end the stream before its end.
		panic(http.ErrAbortHandler)
	default:
		w.Header().Del("Content-Disposition")
		s.usageFailed(w, err)
	}
}

func csvLine(row usage.Row) []string {
	return []string{row.At.UTC().Format(time.RFC3339), safeCell(row.Session), safeCell(row.Task), strconv.Itoa(row.Line), safeCell(row.Person),
		safeCell(row.Team), safeCell(row.Project), safeCell(row.Agent), safeCell(row.Engine), safeCell(row.Provider), safeCell(row.Credential),
		safeCell(row.Model), count(row.InputTokens), count(row.OutputTokens), count(row.CachedTokens), cost(row.USD), strconv.Itoa(row.Status),
		strconv.FormatInt(row.TookMS, 10), safeCell(row.Refused), safeCell(row.Translated)}
}

func count(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}

// cost is a call's cost in dollars, and empty for a call with no price, which is not a call that cost nothing.
func cost(usd *float64) string {
	if usd == nil {
		return ""
	}
	return strconv.FormatFloat(*usd, 'f', -1, 64)
}

// safeCell keeps a spreadsheet from running a cell as a formula: text that begins with a character that starts one is
// given a quote in front, which a spreadsheet shows as nothing.
func safeCell(text string) string {
	if text != "" && strings.ContainsRune("=+-@\t\r", rune(text[0])) {
		return "'" + text
	}
	return text
}
