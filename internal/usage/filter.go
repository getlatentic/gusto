package usage

import (
	"strings"
)

const fieldSeparator = "\x1f"

// searchText is what Search looks in: the fields it covers, lower-cased, so that a match is one
// substring test on one column in either database, which takes % and _ as the characters they are.
func (r Row) searchText() string {
	return strings.ToLower(strings.Join([]string{r.Session, r.Person, r.Agent, r.Engine, r.Provider, r.Model}, fieldSeparator))
}

// where is the WHERE clause that selects the rows a filter matches, with its arguments, or nothing at all
// for the empty filter.
func (s *store) where(f Filter) (string, []any) {
	conds, args := s.conditions(f)
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *store) conditions(f Filter) ([]string, []any) {
	var conds []string
	var args []any
	if !f.From.IsZero() {
		conds, args = append(conds, "called_at >= ?"), append(args, s.db.TimeArg(f.From))
	}
	if !f.To.IsZero() {
		conds, args = append(conds, "called_at < ?"), append(args, s.db.TimeArg(f.To))
	}
	for _, e := range []struct{ column, value string }{{"person", f.Person}, {"team", f.Team}, {"project", f.Project},
		{"engine", f.Engine}, {"agent", f.Agent}, {"provider", f.Provider}, {"model", f.Model}} {
		if e.value != "" {
			conds, args = append(conds, e.column+" = ?"), append(args, e.value)
		}
	}
	if f.Search != "" {
		conds, args = s.appendSearch(conds, args, f.Search)
	}
	return conds, args
}

func (s *store) appendSearch(conds []string, args []any, search string) ([]string, []any) {
	if strings.Contains(search, fieldSeparator) {
		return append(conds, "1 = 0"), args
	}
	return append(conds, s.fl.contains), append(args, strings.ToLower(search))
}
