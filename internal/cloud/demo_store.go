package cloud

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// endStatements take away what an ended demo had, the organization's record last, and each ? in them is the demo's
// slug. A person in the demo who is in another organization too keeps their account; one who was only in the demo
// goes with it.
var endStatements = []string{
	`DELETE FROM invitations WHERE org = ?`,
	`DELETE FROM workspace_members WHERE org = ?`,
	`DELETE FROM people WHERE email IN (SELECT m.email FROM memberships m WHERE m.org = ?
		AND NOT EXISTS (SELECT 1 FROM memberships o WHERE o.email = m.email AND o.org <> ?))`,
	`DELETE FROM memberships WHERE org = ?`,
	`DELETE FROM billing WHERE org = ?`,
	`DELETE FROM runtime WHERE slug = ?`,
	`DELETE FROM workspaces WHERE org = ?`,
	`DELETE FROM orgs WHERE slug = ?`,
}

func slugs(statement, slug string) []any {
	args := make([]any, strings.Count(statement, "?"))
	for i := range args {
		args[i] = slug
	}
	return args
}

// TrashDir is where an organization's directory goes when this process may not remove all of it: files an
// agent's container left as another user. A task that runs as root empties it.
func (s *Store) TrashDir() string { return filepath.Join(s.root, "trash") }

// removeOrgDir removes an organization's directory. What this process may not remove is moved to the
// trash whole, so an organization is never kept because of a file it cannot delete.
func (s *Store) removeOrgDir(slug string) error {
	dir := s.OrgDir(slug)
	err := os.RemoveAll(dir)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	if err := os.MkdirAll(s.TrashDir(), 0o700); err != nil {
		return err
	}
	to := filepath.Join(s.TrashDir(), slug+"-"+strconv.FormatInt(s.now().UnixNano(), 10))
	if err := os.Rename(dir, to); err != nil {
		return err
	}
	slog.Warn("an organization's directory held files this process may not remove, and went to the trash", "org", slug, "trash", to)
	return nil
}

var errNoOrg = fmt.Errorf("there is no such organization: %w", fs.ErrNotExist)

// changeDemo reads an organization under its lock, lets change alter the demo's record, and keeps the result, and the change
// to its lead that lead makes, when it is not nil, in the same transaction, so the lead never says other than the demo.
func (s *Store) changeDemo(ctx context.Context, slug string, change func(*Org) error, lead func(context.Context, dbkit.Tx, Org) error) (Org, error) {

	var org Org
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "org:"+slug); err != nil {
			return err
		}
		var found bool
		var err error
		if org, found, err = fetchOrg(ctx, tx, slug); err != nil || !found {
			if err == nil {
				err = errNoOrg
			}
			return err
		}
		if err := change(&org); err != nil {
			return err
		}
		if err := s.saveDemo(ctx, tx, org); err != nil {
			return err
		}
		if lead == nil {
			return nil
		}
		return lead(ctx, tx, org)
	})
	return org, err
}

func (s *Store) saveDemo(ctx context.Context, tx dbkit.Tx, org Org) error {
	d := org.Demo
	_, err := tx.ExecContext(ctx, `UPDATE orgs SET demo_ready = ?, demo_expires = ?, demo_extended = ?, demo_stopped = ?, demo_ending = ? WHERE slug = ?`,
		s.when(d.Ready), s.when(d.Expires), d.Extended, d.Stopped, d.Ending, org.Slug)
	return err
}

// DemoReady starts a demo's time, when it is not started: the controller asks when the demo's server first answers, and the
// visitor's waiting page when it finds it ready, and the time starts once. A demo that was stopped or is being ended does not
// start. An organization that is not a demo is ErrNotDemo.
func (s *Store) DemoReady(ctx context.Context, slug string) (Demo, error) {
	org, err := s.changeDemo(ctx, slug, func(org *Org) error {
		if org.Demo == nil {
			return ErrNotDemo
		}
		if org.Demo.Ready.IsZero() && !org.Demo.Stopped && !org.Demo.Ending {
			now := s.now().UTC().Truncate(time.Second)
			org.Demo.Ready, org.Demo.Expires = now, now.Add(s.set.Demo.Length)
		}
		return nil
	}, func(ctx context.Context, tx dbkit.Tx, org Org) error {
		if org.Demo.Ready.IsZero() {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE leads SET ready = ? WHERE slug = ? AND ready IS NULL`, s.stamp(org.Demo.Ready), slug)
		return err
	})
	if err != nil {
		return Demo{}, err
	}
	return *org.Demo, nil
}

// ExtendDemo gives a demo the one extension it may have, while it still has time.
func (s *Store) ExtendDemo(ctx context.Context, slug string) (Demo, error) {
	org, err := s.changeDemo(ctx, slug, func(org *Org) error {
		demo := org.Demo
		if demo == nil || demo.Extended || demo.Stopped || demo.Ending || s.set.Demo.Extend <= 0 || demo.Expires.IsZero() || !s.now().Before(demo.Expires) {
			return ErrNoExtension
		}
		demo.Expires, demo.Extended = demo.Expires.Add(s.set.Demo.Extend), true
		return nil
	}, func(ctx context.Context, tx dbkit.Tx, _ Org) error {
		_, err := tx.ExecContext(ctx, `UPDATE leads SET extended = ? WHERE slug = ?`, true, slug)
		return err
	})
	if err != nil {
		return Demo{}, err
	}
	return *org.Demo, nil
}

// StopDemo makes a demo due now, for a visitor who is finished with it.
func (s *Store) StopDemo(ctx context.Context, slug string) error {
	_, err := s.changeDemo(ctx, slug, func(org *Org) error {
		if org.Demo == nil {
			return ErrNotDemo
		}
		org.Demo.Stopped = true
		return nil
	}, nil)
	return err
}

// MarkEnding records that the sweep has begun to end a demo, before it stops anything, so a gusto cloud server that starts again
// partway does not make the demo's server again: the mark is in the database, where a crash does not lose it.
func (s *Store) MarkEnding(ctx context.Context, slug string) error {
	_, err := s.changeDemo(ctx, slug, func(org *Org) error {
		if org.Demo == nil {
			return ErrNotDemo
		}
		org.Demo.Ending = true
		return nil
	}, nil)
	return err
}

// DemoLeft is a demo's record and how long it has, and whether the organization is a demo at all. A demo that is not
// ready has its whole time still, which is not counting down.
func (s *Store) DemoLeft(ctx context.Context, slug string) (Demo, time.Duration, bool) {
	org, found, err := s.Org(ctx, slug)
	if err != nil || !found || org.Demo == nil {
		return Demo{}, 0, false
	}
	if org.Demo.Expires.IsZero() {
		return *org.Demo, s.set.Demo.Length, true
	}
	return *org.Demo, max(org.Demo.Expires.Sub(s.now()), 0), true
}

// DemosDue are the demos whose time is up, or that were never ready and have waited as long as they may.
func (s *Store) DemosDue(ctx context.Context) (map[string]string, error) {
	orgs, err := collect(ctx, s.db, scanOrg, `SELECT `+orgColumns+` FROM orgs WHERE is_demo ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	now := s.now()
	due := map[string]string{}
	for _, org := range orgs {
		switch {
		case org.Demo.Stopped:
			due[org.Slug] = "the visitor ended it"
		case !org.Demo.Expires.IsZero() && !now.Before(org.Demo.Expires):
			due[org.Slug] = "its time was up"
		case org.Demo.Ready.IsZero() && now.Sub(org.Created) > s.set.Demo.patience():
			due[org.Slug] = "it was never ready"
		case org.Demo.Ending:
			due[org.Slug] = "its ending was cut off"
		}
	}
	return due, nil
}

// EndDemo deletes a demo: its people, its data and its record, and keeps the lead with why it ended. The
// organization's server and computers are stopped before this, since nothing here reaches them. The data goes
// first and the record in one step after it, so a failure part way leaves a demo that is found and tried again.
func (s *Store) EndDemo(ctx context.Context, slug, reason string) error {
	org, found, err := s.Org(ctx, slug)
	if err != nil || !found || org.Demo == nil {
		if err == nil {
			err = errors.New("that is not a demo")
		}
		return err
	}
	if err := s.removeOrgDir(slug); err != nil {
		return err
	}

	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "org:"+slug); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE leads SET ended = ?, reason = ? WHERE slug = ? AND ended IS NULL`, s.stamp(s.now().UTC().Truncate(time.Second)), reason, slug); err != nil {
			return err
		}
		for _, statement := range endStatements {
			if _, err := tx.ExecContext(ctx, statement, slugs(statement, slug)...); err != nil {
				return err
			}
		}
		return nil
	})
}
