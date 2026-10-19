package cloud

import (
	"context"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Report is what `gusto cloud migrate` says of the accounts database: where it is, the schema it is at
// and the newest this release knows, how many records it holds, and the import of the old files.
type Report struct {
	Database string
	Version  int
	Latest   int
	Counts   map[string]int
	Import   *ImportRecord
	// Files is how many records of each kind the old files under the root hold that an import would load.
	Files map[string]int
}

// countedTables are the tables whose rows the report counts, under the words people read.
var countedTables = []struct{ name, table string }{
	{"organisations", "orgs"}, {"people", "people"}, {"invitations", "invitations"}, {"links", "links"},
	{"leads", "leads"}, {"runtime", "runtime"},
}

// Inspect reports on the accounts database without changing it: it makes no file and no schema and runs no migration.
func Inspect(ctx context.Context, root string, set Settings) (Report, error) {
	current, latest, err := dbkit.Version(ctx, set.DatabaseURL, root, databaseFile, migrations)
	report := Report{Version: current, Latest: latest, Counts: map[string]int{}, Files: oldRecords(root, time.Now())}
	if err != nil || current == 0 {
		return report, err
	}
	db, err := dbkit.Open(ctx, set.DatabaseURL, root, databaseFile)
	if err != nil {
		return report, err
	}
	defer db.Close()
	report.Database = db.URLForLog()
	if current != latest {
		return report, nil
	}
	return report, fillReport(ctx, db, &report)
}

func fillReport(ctx context.Context, db *dbkit.DB, report *Report) error {
	s := &Store{db: db}
	for _, each := range countedTables {
		n, err := count(ctx, db, `SELECT count(*) FROM `+each.table)
		if err != nil {
			return err
		}
		report.Counts[each.name] = n
	}
	record, found, err := s.importRecord(ctx, db)
	if found {
		report.Import = &record
	}
	return err
}

func oldRecords(root string, now time.Time) map[string]int {
	old := readLegacy(root, now)
	return map[string]int{"organisations": len(old.orgs), "people": len(old.people), "invitations": len(old.invitations),
		"links": len(old.links), "leads": len(old.leads), "runtime": len(old.runtimes)}
}

// ImportNow opens the accounts database, makes its schema current, and loads the old files into it when it has
// not had them, without waiting for a front door to start. It says whether this call did the import, and what the
// import was.
func ImportNow(ctx context.Context, root string, set Settings) (record ImportRecord, ran bool, err error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return ImportRecord{}, false, err
	}
	db, err := dbkit.Open(ctx, set.DatabaseURL, root, databaseFile)
	if err != nil {
		return ImportRecord{}, false, err
	}
	defer db.Close()
	s, err := newStore(db, root, set)
	if err != nil {
		return ImportRecord{}, false, err
	}
	return s.importOnce(ctx)
}
