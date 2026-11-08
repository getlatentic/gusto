package web

import (
	"net/url"
	"strings"
)

// askedBy is the heading of the column that names who asked.
const askedBy = "Asked by"

// describeRows fills in what the index does not hold: each conversation's title, who asked for its latest turn, and whose
// agent it is, as the organization names them. It says how many people asked.
func (s *Server) describeRows(rows []usageRow) int {
	askers := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		row.Harness = s.harnessOf(row.Engine)
		row.Owner = s.whoReads(row.Person)
		row.Asked = row.Owner
		if task, found := s.index.Get(strings.TrimPrefix(row.Record, "/jobs/")); found {
			row.Title, row.Asked = task.Brief, s.whoReads(task.Asked)
		}
		askers[row.Asked] = true
	}
	return len(askers)
}

// whoReads is a person by the name the organization gives them, and a token or the shared sign-in as what it is.
func (s *Server) whoReads(who string) string {
	if strings.Contains(who, "@") {
		return s.personName(who)
	}
	return askerOf(who)
}

// filteredBeyond says the table is narrowed past the view it is on and, for a person, past their own calls.
func filteredBeyond(values url.Values, person string) bool {
	for _, name := range usageFields {
		if name == "state" || (name == "person" && person != "") {
			continue
		}
		if values.Get(name) != "" {
			return true
		}
	}
	return false
}

// clearedOf is the table with no filter, on the view it is on.
func clearedOf(values url.Values, base string) string {
	if state := values.Get("state"); state != "" {
		return base + "?" + url.Values{"state": {state}}.Encode()
	}
	return base
}
