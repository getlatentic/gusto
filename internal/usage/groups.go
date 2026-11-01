package usage

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

// ErrBadGroup is a Groups call asked to add calls up by something other than ByPerson or ByEngine.
var ErrBadGroup = errors.New("usage: unknown grouping")

// What Groups adds calls up by: the column of the index that holds it.
const (
	ByPerson = "person"
	ByEngine = "engine"
)

// Group is the calls that share one value of a column, added up. Agents is how many of the agents people set up
// made them.
type Group struct {
	Value  string `json:"value"`
	Agents int64  `json:"agents"`
	Totals
}

// SortGroups puts groups in the order a page lists them: the most spent first, then by value.
func SortGroups(groups []Group) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].USD != groups[j].USD {
			return groups[i].USD > groups[j].USD
		}
		return groups[i].Value < groups[j].Value
	})
}

func (s *store) Groups(ctx context.Context, filter Filter, by string) ([]Group, error) {
	if by != ByPerson && by != ByEngine {
		return nil, ErrBadGroup
	}
	where, args := s.where(filter)
	query := `SELECT ` + by + `, count(DISTINCT NULLIF(agent, '')), ` + totalsColumns + ` FROM usage_calls` + where + ` GROUP BY ` + by
	var out []Group
	err := s.query(ctx, s.db, query, args, func(r *sql.Rows) error {
		var g Group
		if err := r.Scan(append([]any{&g.Value, &g.Agents}, g.destinations()...)...); err != nil {
			return err
		}
		out = append(out, g)
		return nil
	})
	if err != nil {
		return nil, err
	}
	SortGroups(out)
	return out, nil
}
