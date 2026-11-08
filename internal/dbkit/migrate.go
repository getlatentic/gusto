package dbkit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var tableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// Migrations is the schema a component brings: SQL files in FS named <Dir>/sqlite/0001_name.sql and
// <Dir>/postgres/0001_name.sql, one set for each dialect, numbered from 1 with none missing and applied in
// version order. Table is the version table the component keeps, and Scope names the component for locks and
// messages.
type Migrations struct {
	Scope string
	Table string
	FS    fs.FS
	Dir   string
}

type migration struct {
	version int
	name    string
	sql     string
}

func (m Migrations) validate() error {
	if m.Scope == "" || m.FS == nil || !tableName.MatchString(m.Table) {
		return errors.New("dbkit: migrations need a scope, a file system and a plain table name")
	}
	return nil
}

// load lists a dialect's migrations in the order they apply.
func (m Migrations) load(d Dialect) ([]migration, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	dir := path.Join(m.Dir, string(d))
	entries, err := fs.ReadDir(m.FS, dir)
	if err != nil {
		return nil, fmt.Errorf("%s: read migrations: %w", m.Scope, err)
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		mig, err := m.read(dir, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, mig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, checkSequence(m.Scope, out)
}

func (m Migrations) read(dir, name string) (migration, error) {
	stem, _, _ := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
	v, err := strconv.Atoi(stem)
	if err != nil || v < 1 {
		return migration{}, fmt.Errorf("%s: migration %s has no version number", m.Scope, name)
	}
	body, err := fs.ReadFile(m.FS, path.Join(dir, name))
	if err != nil {
		return migration{}, fmt.Errorf("%s: read migration %s: %w", m.Scope, name, err)
	}
	return migration{version: v, name: name, sql: string(body)}, nil
}

// checkSequence holds a set to 1, 2, 3 and on, with none missing, so that a migration added below one already applied is
// a gap or a duplicate here and not a version a database passes over.
func checkSequence(scope string, ms []migration) error {
	if len(ms) == 0 {
		return fmt.Errorf("%s: no migrations", scope)
	}
	for i, mig := range ms {
		if i > 0 && mig.version == ms[i-1].version {
			return fmt.Errorf("%s: two migrations have version %d", scope, mig.version)
		}
		if mig.version != i+1 {
			return fmt.Errorf("%s: migration %s is numbered %d where %d is next: migrations are numbered from 1 with none missing",
				scope, mig.name, mig.version, i+1)
		}
	}
	return nil
}

// Migrate applies the migrations the database has not had, each in its own transaction. On Postgres an
// advisory lock keyed by Scope makes a second process wait for the first; on SQLite the write lock does.
// A database that is at a newer version than the migrations know is refused.
func Migrate(ctx context.Context, db *DB, m Migrations) error {
	ms, err := m.load(db.dialect)
	if err != nil {
		return err
	}
	if err := db.InTx(ctx, func(tx Tx) error { return checkApplied(ctx, db, tx, m, ms) }); err != nil {
		return err
	}
	for _, mig := range ms {
		if err := db.InTx(ctx, func(tx Tx) error { return applyOne(ctx, db, tx, m, mig) }); err != nil {
			return err
		}
	}
	return nil
}

// checkApplied refuses a database that is newer than the migrations, that passed over one of them, or that had a different
// migration under one's number.
func checkApplied(ctx context.Context, db *DB, tx Tx, m Migrations, ms []migration) error {
	applied, err := appliedVersions(ctx, tx, db.dialect, m.Table)
	if err != nil {
		return fmt.Errorf("%s: read schema version: %w", m.Scope, err)
	}
	current := 0
	for version := range applied {
		current = max(current, version)
	}
	if latest := ms[len(ms)-1].version; current > latest {
		return fmt.Errorf("%s: the database is at schema version %d and this gusto knows up to %d; upgrade gusto before using it",
			m.Scope, current, latest)
	}
	for _, mig := range ms {
		name, had := applied[mig.version]
		switch {
		case had && name != mig.name:
			return fmt.Errorf("%s: the database had %s as migration %d, and this gusto's is %s; nothing has been changed",
				m.Scope, name, mig.version, mig.name)
		case !had && mig.version < current:
			return fmt.Errorf("%s: the database is at schema version %d and never had migration %s, which is numbered below it; "+
				"nothing has been changed", m.Scope, current, mig.name)
		}
	}
	return nil
}

func applyOne(ctx context.Context, db *DB, tx Tx, m Migrations, mig migration) error {
	if err := db.lock(ctx, tx, "migrate:"+m.Scope); err != nil {
		return fmt.Errorf("%s: migrate: %w", m.Scope, err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+m.Table+` (
		version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at BIGINT NOT NULL)`); err != nil {
		return fmt.Errorf("%s: migrate: %w", m.Scope, err)
	}
	var had bool
	if err := tx.QueryRowContext(ctx, `SELECT count(*) > 0 FROM `+m.Table+` WHERE version = ?`, mig.version).Scan(&had); err != nil {
		return fmt.Errorf("%s: read schema version: %w", m.Scope, err)
	}
	if had {
		return nil
	}
	if _, err := tx.ExecContext(ctx, mig.sql); err != nil {
		return fmt.Errorf("%s: migration %s: %w", m.Scope, mig.name, err)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO `+m.Table+` (version, name, applied_at) VALUES (?, ?, ?)`,
		mig.version, mig.name, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("%s: migration %s: %w", m.Scope, mig.name, err)
	}
	return nil
}

// currentVersion is the newest migration the database has had, and 0 for one that has had none or whose
// version table does not exist yet.
func currentVersion(ctx context.Context, q Querier, d Dialect, table string) (int, error) {
	exists, err := tableExists(ctx, q, d, table)
	if err != nil || !exists {
		return 0, err
	}
	var v int
	err = q.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM `+table).Scan(&v)
	return v, err
}

// appliedVersions is every migration the database has had, by version, and none for one whose version table does not exist yet.
func appliedVersions(ctx context.Context, q Querier, d Dialect, table string) (map[int]string, error) {
	applied := map[int]string{}
	exists, err := tableExists(ctx, q, d, table)
	if err != nil || !exists {
		return applied, err
	}
	rows, err := q.QueryContext(ctx, `SELECT version, name FROM `+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, err
		}
		applied[version] = name
	}
	return applied, rows.Err()
}

func tableExists(ctx context.Context, q Querier, d Dialect, table string) (bool, error) {
	var query string
	if d == Postgres {
		query = `SELECT to_regclass(?) IS NOT NULL`
	} else {
		query = `SELECT count(*) > 0 FROM sqlite_master WHERE type = 'table' AND name = ?`
	}
	var found bool
	err := q.QueryRowContext(ctx, query, table).Scan(&found)
	return found, err
}

// Version reports the schema version a database is at and the newest the migrations know, without
// changing the database: it never creates a file, a directory or a schema. A database that does not exist
// yet is at 0.
func Version(ctx context.Context, databaseURL, dir, file string, m Migrations) (current, latest int, err error) {
	t, err := parseTarget(databaseURL, dir, file)
	if err != nil {
		return 0, 0, err
	}
	ms, err := m.load(t.dialect)
	if err != nil {
		return 0, 0, err
	}
	latest = ms[len(ms)-1].version
	if t.dialect == SQLite {
		if _, statErr := os.Stat(t.path); errors.Is(statErr, os.ErrNotExist) {
			return 0, latest, nil
		}
	}
	db, err := connect(ctx, t)
	if err != nil {
		return 0, latest, err
	}
	defer db.Close()
	current, err = currentVersion(ctx, db, db.dialect, m.Table)
	if err != nil {
		err = t.redact(fmt.Errorf("%s: read schema version: %w", m.Scope, err))
	}
	return current, latest, err
}
