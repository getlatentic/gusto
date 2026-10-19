package usage

import "github.com/getlatentic/gusto/internal/dbkit"

// flavor holds the two places where the queries of this package differ between databases.
type flavor struct {
	// contains is true of a row whose search column holds the one argument.
	contains string
	// filteredGroup is how sessions are grouped when a filter picks the rows. SQLite would otherwise walk the
	// session index and fetch every row from the table to test it, where a scan of the table is far quicker.
	filteredGroup string
}

func flavorOf(d dbkit.Dialect) flavor {
	if d == dbkit.Postgres {
		return flavor{contains: "strpos(search, ?) > 0", filteredGroup: "session"}
	}
	return flavor{contains: "instr(search, ?) > 0", filteredGroup: "+session"}
}
