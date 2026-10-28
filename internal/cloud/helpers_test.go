package cloud

import (
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

// The tests that need a database run once on SQLite and, when GUSTO_TEST_POSTGRES_URL is set, once on Postgres in a
// fresh schema. The first store a test opens gets the database dbkittest made, and any other store the test opens
// gets a fresh database of the same kind.
var (
	testDialect dbkit.Dialect
	spareDB     *dbkit.DB
)

func onEachDatabase(t *testing.T, body func(t *testing.T)) {
	t.Helper()
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		testDialect, spareDB = db.Dialect(), db
		body(t)
	})
}

func freshDatabase(t testing.TB) *dbkit.DB {
	t.Helper()
	if db := spareDB; db != nil {
		spareDB = nil
		return db
	}
	if testDialect == dbkit.Postgres {
		return dbkittest.Postgres(t)
	}
	return dbkittest.SQLite(t)
}

func open(t testing.TB, set Settings) *Store {
	t.Helper()
	s, err := openOn(freshDatabase(t), filepath.Join(t.TempDir(), "cloud"), set)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// only is the membership of an address that is in one organization, which is what most tests make.
func (s *Store) only(email string) (Person, bool, error) {
	all, err := s.Memberships(email)
	if err != nil || len(all) != 1 {
		return Person{}, false, err
	}
	return all[0], true, nil
}
