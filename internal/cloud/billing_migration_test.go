package cloud

import (
	"context"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
	"github.com/getlatentic/gusto/internal/dbkit"
)

// migrationsThrough is the cloud schema as it was at a version.
func migrationsThrough(t *testing.T, version int) dbkit.Migrations {
	t.Helper()
	only := fstest.MapFS{}
	for _, dialect := range []string{"sqlite", "postgres"} {
		dir := filepath.ToSlash(filepath.Join("migrations", dialect))
		entries, err := fs.ReadDir(migrationFiles, dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if n, _ := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0]); n > version {
				continue
			}
			text, err := fs.ReadFile(migrationFiles, dir+"/"+entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			only[dir+"/"+entry.Name()] = &fstest.MapFile{Data: text}
		}
	}
	return dbkit.Migrations{Scope: migrations.Scope, Table: migrations.Table, FS: only, Dir: migrations.Dir}
}

func TestAStripeSubscriptionOfAnEarlierSchemaBecomesAPlanThatSaysTheSame(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		ctx := context.Background()
		db := freshDatabase(t)
		if err := dbkit.Migrate(ctx, db, migrationsThrough(t, 8)); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
		stripe := []struct {
			org, status string
			state       payments.State
			paidOnce    bool
		}{
			{"a-com", "active", payments.Active, true}, {"b-com", "trialing", payments.Active, true}, {"c-com", "past_due", payments.PastDue, true},
			{"d-com", "canceled", payments.Suspended, true}, {"e-com", "unpaid", payments.Suspended, true}, {"f-com", "incomplete", payments.Pending, false},
			{"g-com", "incomplete_expired", payments.Suspended, false},
		}
		for _, each := range stripe {
			if _, err := db.ExecContext(ctx, `INSERT INTO orgs (slug, name, admin, created) VALUES (?, ?, ?, ?)`, each.org, each.org, "ada@"+each.org, db.TimeArg(at)); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO billing (org, customer, subscription, item, status, seats, event_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				each.org, "cus_"+each.org, "sub_"+each.org, "si_"+each.org, each.status, 3, db.TimeArg(at)); err != nil {
				t.Fatal(err)
			}
		}
		s, err := openOn(db, filepath.Join(t.TempDir(), "cloud"), Settings{})
		if err != nil {
			t.Fatal(err)
		}
		for _, each := range stripe {
			plan, found, err := s.Plan(ctx, each.org)
			if err != nil || !found {
				t.Fatalf("%s: %v %v", each.org, found, err)
			}
			if plan.Provider != "stripe" || plan.Ref != "sub_"+each.org || plan.Customer != "cus_"+each.org || plan.Handle != "si_"+each.org ||
				plan.State != each.state || plan.PaidOnce != each.paidOnce || plan.Seats != 3 || plan.Fixed || !plan.SourceTime.Equal(at) {
				t.Errorf("%s (%s): %+v", each.org, each.status, plan)
			}
		}
		if _, err := db.ExecContext(ctx, `SELECT 1 FROM billing`); err == nil {
			t.Error("the table the plans replace is still there")
		}
	})
}
