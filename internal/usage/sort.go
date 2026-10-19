package usage

import (
	"errors"
	"fmt"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

var errTimeColumn = dbkit.ErrTimeColumn

// ErrBadSort is a Page.Sort that no listing offers.
var ErrBadSort = errors.New("usage: unknown sort")

// sortSpec is how sessions are ranked: an aggregate over a session's matching calls, always whole numbers so
// that adding the same calls in any order gives the same rank and a keyset cursor never lands between two
// near-equal sums.
type sortSpec struct {
	expr string
	// arg turns a rank held in a cursor into the form the database compares it in.
	arg func(db *dbkit.DB, v int64) any
}

func plainRank(_ *dbkit.DB, v int64) any { return v }

// Money is ranked in billionths of a dollar; a session with no measured call ranks below every measured one,
// even one that cost nothing.
const costRank = `COALESCE(CAST(SUM(CAST(ROUND(usd * 1000000000) AS BIGINT)) AS BIGINT), -1)`

var sortSpecs = map[string]sortSpec{
	"last":   {"MAX(called_at)", func(db *dbkit.DB, v int64) any { return db.TimeArg(time.Unix(0, v)) }},
	"cost":   {costRank, plainRank},
	"calls":  {"count(*)", plainRank},
	"tokens": {"CAST(COALESCE(SUM(input_tokens), 0) + COALESCE(SUM(output_tokens), 0) AS BIGINT)", plainRank},
}

// sortFor finds a sort by name, where the empty name is the default, last.
func sortFor(name string) (string, sortSpec, error) {
	if name == "" {
		name = "last"
	}
	spec, ok := sortSpecs[name]
	if !ok {
		return "", sortSpec{}, fmt.Errorf("%w %q; use last, cost, calls or tokens", ErrBadSort, name)
	}
	return name, spec, nil
}

// rankCol reads a rank, which is a time in nanoseconds for the last sort.
type rankCol int64

func (k *rankCol) Scan(v any) error {
	switch x := v.(type) {
	case int64:
		*k = rankCol(x)
	case time.Time:
		*k = rankCol(x.UnixNano())
	default:
		return errTimeColumn
	}
	return nil
}
