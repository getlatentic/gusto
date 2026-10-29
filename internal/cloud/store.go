package cloud

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

// Settings are what an operator chooses about sign-ups.
type Settings struct {
	// AllowFreeMail lets addresses at free mail providers sign up, which a
	// demonstration wants and a service for companies does not.
	AllowFreeMail bool
	// RefusedDomains are more domains to turn away, beside the built-in ones.
	RefusedDomains []string
	// PerDomain is how many organizations one email domain may make.
	PerDomain int
	Limits    Limits
	// Demo, when it is set, makes the front door offer a demo.
	Demo DemoSettings
	// DatabaseURL is where the accounts are kept: empty for SQLite in cloud.db under the root, or a
	// postgres:// address whose search_path parameter names the schema.
	DatabaseURL string
}

// Store is the front door's accounts, kept in a database that any number of
// front doors may share: SQLite in a file under the root by default, or
// Postgres. Everything a name or a count must be right about is decided inside
// a transaction, so two front doors cannot both take an address or accept a
// link. An organization's own directory under the root stays a directory.
type Store struct {
	root    string
	set     Settings
	refused map[string]bool
	db      *dbkit.DB
	now     func() time.Time
}

// Open opens the database, brings its schema up to date and, the first time a
// database is opened, loads the files an earlier release kept under the root.
func Open(root string, set Settings) (*Store, error) {
	ctx := context.Background()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	db, err := openDatabase(ctx, set.DatabaseURL, root)
	if err != nil {
		return nil, err
	}
	s, err := openOn(db, root, set)
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// openOn makes a store of an open database and imports the old files when the database has not had them.
func openOn(db *dbkit.DB, root string, set Settings) (*Store, error) {
	s, err := newStore(db, root, set)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.importOnce(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// newStore makes the directories the store keeps its files in and brings the schema up to date.
func newStore(db *dbkit.DB, root string, set Settings) (*Store, error) {
	if set.PerDomain <= 0 {
		set.PerDomain = 3
	}
	if set.Limits == (Limits{}) {
		set.Limits = DefaultLimits()
	}
	if set.Limits.Paid == (quota.Quota{}) {
		set.Limits.Paid = DefaultLimits().Paid
	}
	if set.Demo.Enabled() && set.Demo.Limits == (quota.Quota{}) {
		set.Demo.Limits = DefaultDemoLimits()
	}
	for _, dir := range []string{"", "orgs", "outbox"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return nil, err
		}
	}
	if err := dbkit.Migrate(context.Background(), db, migrations); err != nil {
		return nil, err
	}
	return &Store{root: root, set: set, refused: setOf(set.RefusedDomains...), db: db, now: time.Now}, nil
}

// Attach is a second program's view of a store another program runs. It changes nothing in the
// database when it opens: it does not migrate it and does not import into it.
func Attach(root string, set Settings) (*Store, error) {
	ctx := context.Background()
	current, latest, err := dbkit.Version(ctx, set.DatabaseURL, root, databaseFile, migrations)
	switch {
	case err != nil:
		return nil, err
	case current == 0:
		return nil, fmt.Errorf("%s holds no accounts: run gusto cloud there first", root)
	case current != latest:
		return nil, fmt.Errorf("the accounts database is at schema version %d and this gusto reads %d: run the same release as gusto cloud", current, latest)
	}
	db, err := dbkit.Open(ctx, set.DatabaseURL, root, databaseFile)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, set: set, refused: setOf(set.RefusedDomains...), db: db, now: time.Now}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Root is the directory the store keeps an organization's files under.
func (s *Store) Root() string { return s.root }

// Database is where the accounts are, with no password in it.
func (s *Store) Database() string { return s.db.URLForLog() }

// OrgDir is where an organization's own files are, and DataDir the directory its
// gusto server is given. Only the organization's server and this process reach
// them, and they are not readable by anyone else on the machine.
func (s *Store) OrgDir(slug string) string  { return filepath.Join(s.root, "orgs", slug) }
func (s *Store) DataDir(slug string) string { return filepath.Join(s.OrgDir(slug), "data") }

// Quota is what an organization may run now.
func (s *Store) Quota(slug string) (quota.Quota, error) {
	var q quota.Quota
	return q, readFile(s.QuotaFile(slug), &q)
}

// Computers is the organization's record of its computers, which its server keeps
// and this reads.
func (s *Store) Computers(slug string) (*computer.Registry, error) {
	return computer.Open(filepath.Join(s.DataDir(slug), "computers"), slug)
}

// QuotaFile is where the organization's server reads its limits.
func (s *Store) QuotaFile(slug string) string { return filepath.Join(s.DataDir(slug), "quota.json") }

func (s *Store) setQuota(slug string, q quota.Quota) error { return quota.Write(s.QuotaFile(slug), q) }
