package usage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

// migrationsFor lists a dialect's migrations in the order they apply.
func migrationsFor(d dialect) ([]migration, error) {
	dir := path.Join("migrations", d.name)
	entries, err := migrationFiles.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		stem, _, _ := strings.Cut(strings.TrimSuffix(e.Name(), ".sql"), "_")
		v, err := strconv.Atoi(stem)
		if err != nil {
			return nil, fmt.Errorf("usage: migration %s has no version number", e.Name())
		}
		body, err := migrationFiles.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// migrate applies the migrations the database has not had, in one transaction. On Postgres an advisory lock
// makes a second process wait for the first.
func migrate(ctx context.Context, db *sql.DB, d dialect, schema string) error {
	ms, err := migrationsFor(d)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usage: migrate: %w", err)
	}
	defer tx.Rollback()
	if err := prepareMigrations(ctx, tx, d, schema); err != nil {
		return err
	}
	current, err := appliedVersion(ctx, tx, d)
	if err != nil {
		return err
	}
	if latest := ms[len(ms)-1].version; current > latest {
		return fmt.Errorf("usage: the database is at schema version %d and this gusto knows up to %d; upgrade gusto before using it", current, latest)
	}
	for _, m := range ms {
		if m.version <= current {
			continue
		}
		if err := apply(ctx, tx, d, m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func prepareMigrations(ctx context.Context, tx *sql.Tx, d dialect, schema string) error {
	if d.lock != "" {
		if _, err := tx.ExecContext(ctx, d.lock); err != nil {
			return fmt.Errorf("usage: migrate: %w", err)
		}
	}
	if schema != "" {
		if _, err := tx.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS "`+schema+`"`); err != nil {
			return fmt.Errorf("usage: create schema %s: %w", schema, err)
		}
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at BIGINT NOT NULL)`)
	if err != nil {
		return fmt.Errorf("usage: migrate: %w", err)
	}
	return nil
}

func apply(ctx context.Context, tx *sql.Tx, d dialect, m migration) error {
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("usage: migration %s: %w", m.name, err)
	}
	_, err := tx.ExecContext(ctx, d.rebind(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`),
		m.version, m.name, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("usage: migration %s: %w", m.name, err)
	}
	return nil
}

// appliedVersion is the newest migration the database has had, and 0 for one that has had none.
func appliedVersion(ctx context.Context, q querier, d dialect) (int, error) {
	var has int
	if err := q.QueryRowContext(ctx, d.hasMigrations).Scan(&has); err != nil {
		return 0, fmt.Errorf("usage: read schema version: %w", err)
	}
	if has == 0 {
		return 0, nil
	}
	var v int
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("usage: read schema version: %w", err)
	}
	return v, nil
}
