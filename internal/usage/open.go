package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const acceptedForms = `use postgres://user:password@host/database, postgresql://..., sqlite:///path/to/file.db, or leave it empty for usage.db in the data directory`

var (
	errTimeColumn = errors.New("usage: a time column held a value of the wrong kind")
	schemaName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)
)

// target is where an index lives, with the password kept apart so that no message can carry it.
type target struct {
	dialect  dialect
	path     string
	pg       *pgx.ConnConfig
	schema   string
	password string
}

// Open opens the index and brings its schema up to date. An empty databaseURL is SQLite in dir; a
// postgres:// or postgresql:// URL is Postgres, whose search_path parameter names the schema to keep the
// tables in; a sqlite:///path URL is SQLite in another file.
func Open(ctx context.Context, databaseURL, dir string) (Store, error) {
	t, err := parseTarget(databaseURL, dir)
	if err != nil {
		return nil, err
	}
	if t.dialect.name == "sqlite" {
		if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
			return nil, fmt.Errorf("usage: make the data directory: %w", err)
		}
	}
	s, err := t.connect(ctx)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, s.w, t.dialect, t.schema); err != nil {
		s.Close()
		return nil, t.redact(err)
	}
	return s, nil
}

// Version reports the schema version a database is at and the newest this program knows, without changing
// the database. A database that does not exist yet is at 0.
func Version(ctx context.Context, databaseURL, dir string) (current, latest int, err error) {
	t, err := parseTarget(databaseURL, dir)
	if err != nil {
		return 0, 0, err
	}
	ms, err := migrationsFor(t.dialect)
	if err != nil {
		return 0, 0, err
	}
	latest = ms[len(ms)-1].version
	if t.dialect.name == "sqlite" {
		if _, statErr := os.Stat(t.path); errors.Is(statErr, os.ErrNotExist) {
			return 0, latest, nil
		}
	}
	s, err := t.connect(ctx)
	if err != nil {
		return 0, latest, err
	}
	defer s.Close()
	current, err = appliedVersion(ctx, s.r, t.dialect)
	return current, latest, t.redact(err)
}

func parseTarget(databaseURL, dir string) (target, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return sqliteTarget(filepath.Join(dir, "usage.db")), nil
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		return target{}, fmt.Errorf("usage: the database URL is not valid; %s", acceptedForms)
	}
	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
		return postgresTarget(u)
	case "sqlite":
		return sqliteURLTarget(u, dir), nil
	}
	return target{}, fmt.Errorf("usage: database URL scheme %q is not supported; %s", u.Scheme, acceptedForms)
}

func sqliteTarget(path string) target {
	return target{dialect: sqliteDialect, path: path}
}

func sqliteURLTarget(u *url.URL, dir string) target {
	path := u.Host + u.Path
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if path == "" {
		path = filepath.Join(dir, "usage.db")
	}
	return sqliteTarget(filepath.FromSlash(path))
}

func postgresTarget(u *url.URL) (target, error) {
	q := u.Query()
	schema := q.Get("search_path")
	q.Del("search_path")
	if schema != "" && !schemaName.MatchString(schema) {
		return target{}, errors.New("usage: search_path must name one schema, of letters, digits and underscores")
	}
	clean := *u
	clean.RawQuery = q.Encode()
	cfg, err := pgx.ParseConfig(clean.String())
	if err != nil {
		return target{}, fmt.Errorf("usage: the database URL is not valid; %s", acceptedForms)
	}
	if schema != "" {
		cfg.RuntimeParams["search_path"] = schema
	}
	return target{dialect: postgresDialect, pg: cfg, schema: schema, password: cfg.Password}, nil
}

func (t target) connect(ctx context.Context) (*store, error) {
	if t.dialect.name == "postgres" {
		return t.connectPostgres(ctx)
	}
	return t.connectSQLite(ctx)
}

func (t target) connectPostgres(ctx context.Context) (*store, error) {
	db := stdlib.OpenDB(*t.pg)
	db.SetMaxOpenConns(16)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, t.redact(fmt.Errorf("usage: cannot reach the database: %w", err))
	}
	return &store{d: t.dialect, w: db, r: db}, nil
}

func (t target) connectSQLite(ctx context.Context) (*store, error) {
	const pragmas = "_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	w, err := sql.Open("sqlite", t.path+"?"+pragmas+"&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("usage: open %s: %w", t.path, err)
	}
	w.SetMaxOpenConns(1)
	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("usage: open %s: %w", t.path, err)
	}
	r, err := sql.Open("sqlite", t.path+"?"+pragmas+"&_pragma=query_only(1)")
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("usage: open %s: %w", t.path, err)
	}
	r.SetMaxOpenConns(max(4, runtime.NumCPU()))
	return &store{d: t.dialect, w: w, r: r}, nil
}

// redact removes the password from an error, since a driver may quote the connection it was given.
func (t target) redact(err error) error {
	if err == nil || t.password == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), t.password, "xxxxx"))
}
