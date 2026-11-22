package cloud

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// CopyFrom copies the accounts in a SQLite file into the Postgres database the settings name, and says how many rows of
// each table it copied. The file must be at the schema version this release reads, and is only read; the Postgres schema
// is brought to that version and must hold no accounts. It is one transaction, so a copy that fails leaves nothing behind.
// gusto cloud is stopped first, so nothing writes the file while it is read.
func CopyFrom(ctx context.Context, file string, set Settings) (map[string]int, error) {
	absolute, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	source := "sqlite://" + absolute
	current, latest, err := dbkit.Version(ctx, source, "", "", migrations)
	switch {
	case err != nil:
		return nil, err
	case current != latest:
		return nil, fmt.Errorf("%s is at schema version %d and this gusto reads %d: start this release on it once, then copy", file, current, latest)
	}
	from, err := dbkit.Open(ctx, source, "", "")
	if err != nil {
		return nil, err
	}
	defer from.Close()
	to, err := dbkit.Open(ctx, set.DatabaseURL, "", "")
	if err != nil {
		return nil, err
	}
	defer to.Close()
	if to.Dialect() != dbkit.Postgres {
		return nil, fmt.Errorf("GUSTO_DATABASE_URL is where the accounts are copied to, and is a postgres:// address")
	}
	if err := dbkit.Migrate(ctx, to, migrations); err != nil {
		return nil, err
	}
	tables, err := dbkit.Tables(ctx, from, migrations.Table)
	if err != nil {
		return nil, err
	}
	return dbkit.Copy(ctx, from, to, tables)
}
