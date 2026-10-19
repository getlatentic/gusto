package usage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

func (s *store) Facets(ctx context.Context, filter Filter) (Facets, error) {
	var f Facets
	into := f.columns()
	where, args := s.where(filter)
	if where == "" {
		for i, column := range facetColumns {
			values, err := s.valuesOf(ctx, column)
			if err != nil {
				return Facets{}, err
			}
			*into[i] = values
		}
		return f, nil
	}
	return f, s.valuesWhere(ctx, where, args, into)
}

var facetColumns = []string{"person", "team", "project", "engine", "agent", "provider", "model"}

func (f *Facets) columns() []*[]string {
	return []*[]string{&f.People, &f.Teams, &f.Projects, &f.Engines, &f.Agents, &f.Providers, &f.Models}
}

// valuesOf lists the values of a column that are not empty, in byte order. It steps from one value to the next
// through the column's index, so it reads a row for each value and not for each call.
func (s *store) valuesOf(ctx context.Context, column string) ([]string, error) {
	next := `(SELECT MIN(` + column + `) FROM usage_calls WHERE ` + column + ` > %s)`
	query := `WITH RECURSIVE v(x) AS (SELECT ` + fmt.Sprintf(next, `''`) +
		` UNION ALL SELECT ` + fmt.Sprintf(next, `v.x`) + ` FROM v WHERE v.x IS NOT NULL)
		SELECT x FROM v WHERE x IS NOT NULL`
	var out []string
	err := s.query(ctx, s.r, query, nil, func(r *sql.Rows) error {
		var v string
		if err := r.Scan(&v); err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	return out, err
}

// valuesWhere reads the distinct combinations of the matching calls once and sorts out each column's values,
// which is quicker than a pass for each column when a filter has left many calls.
func (s *store) valuesWhere(ctx context.Context, where string, args []any, into []*[]string) error {
	sets := make([]map[string]bool, len(facetColumns))
	for i := range sets {
		sets[i] = map[string]bool{}
	}
	cols := facetColumns
	query := `SELECT DISTINCT ` + strings.Join(cols, ", ") + ` FROM usage_calls` + where
	err := s.query(ctx, s.r, query, args, func(r *sql.Rows) error {
		values := make([]string, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := r.Scan(dest...); err != nil {
			return err
		}
		for i, v := range values {
			if v != "" {
				sets[i][v] = true
			}
		}
		return nil
	})
	for i, set := range sets {
		for v := range set {
			*into[i] = append(*into[i], v)
		}
		sort.Strings(*into[i])
	}
	return err
}
