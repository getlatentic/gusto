package cloud

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const rowsPerInsert = 400

func ignoreNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// insertRows inserts rows that are not there yet, in groups, and says how many went in.
func insertRows(ctx context.Context, tx dbkit.Tx, up dbkit.Upsert, rows [][]any) (int, error) {
	added := 0
	for start := 0; start < len(rows); start += rowsPerInsert {
		group := rows[start:min(start+rowsPerInsert, len(rows))]
		args := make([]any, 0, len(group)*len(up.Columns))
		for _, row := range group {
			args = append(args, row...)
		}
		done, err := tx.ExecContext(ctx, up.SQL(len(group)), args...)
		if err != nil {
			return added, err
		}
		n, err := done.RowsAffected()
		if err != nil {
			return added, err
		}
		added += int(n)
	}
	return added, nil
}

// insertLegacy puts what the old files held into the tables, parents before the rows that point at them.
func (s *Store) insertLegacy(ctx context.Context, tx dbkit.Tx, old legacy) (map[string]int, error) {
	imported := map[string]int{}
	steps := []struct {
		kind string
		up   dbkit.Upsert
		rows [][]any
	}{
		{"organisations", insertOrg, mapRows(old.orgs, s.orgArgs)},
		{"people", insertPerson, mapRows(old.people, s.personArgs)},
		{"invitations", dbkit.Upsert{Table: "invitations", Columns: invitationFields, Conflict: []string{"org", "email"}}, mapRows(old.invitations, s.legacyInvitationArgs)},
		{"leads", insertLead, mapRows(old.leads, s.leadArgs)},
		{"links", insertLink, mapRows(old.links, func(l legacyLink) []any { return s.linkArgs(l.hash, l.link) })},
	}
	for _, step := range steps {
		n, err := insertRows(ctx, tx, step.up, step.rows)
		if err != nil {
			return nil, err
		}
		imported[step.kind] = n
	}
	var err error
	if imported["runtime"], err = s.insertRuntimes(ctx, tx, old); err != nil {
		return nil, err
	}
	if imported["settings"], err = s.insertSettings(ctx, tx, old); err != nil {
		return nil, err
	}
	return imported, nil
}

func mapRows[T any](items []T, args func(T) []any) [][]any {
	rows := make([][]any, len(items))
	for i, item := range items {
		rows[i] = args(item)
	}
	return rows
}

func (s *Store) legacyInvitationArgs(i legacyInvitation) []any {
	return s.invitationArgs(i.org, i.Invitation)
}

// insertRuntimes keeps each organisation's runtime unless another row already holds its slug or its ports, in
// which case that one stands and the controller works the ports out again.
func (s *Store) insertRuntimes(ctx context.Context, tx dbkit.Tx, old legacy) (int, error) {
	added := 0
	for slug, rt := range old.runtimes {
		ok, err := inserted(tx.ExecContext(ctx, `INSERT INTO runtime (slug, state, cell, port, proxy_port, error, restarts, updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			slug, string(rt.State), rt.Cell, rt.Port, rt.ProxyPort, rt.Error, rt.Restarts, s.stamp(rt.Updated)))
		if err != nil {
			return added, err
		}
		if ok {
			added++
		} else {
			old.skipped["runtime conflicting"]++
		}
	}
	return added, nil
}

// insertSettings keeps the demo limits, the pause and the cookie key the old files held, unless the
// database has them already.
func (s *Store) insertSettings(ctx context.Context, tx dbkit.Tx, old legacy) (int, error) {
	values := map[string]string{}
	if old.limits != nil {
		text, err := json.Marshal(old.limits)
		if err != nil {
			return 0, err
		}
		values[settingDemoLimits] = string(text)
	}
	if old.paused {
		values[settingDemosPaused] = "1"
	}
	if old.key != nil {
		values[settingSessionKey] = hex.EncodeToString(old.key)
	}
	added := 0
	for key, value := range values {
		ok, err := inserted(tx.ExecContext(ctx, keepSetting.SQL(1), key, value))
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}
