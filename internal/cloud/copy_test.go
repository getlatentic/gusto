package cloud

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

func TestAccountsInASQLiteFileAreCopiedIntoPostgresAndReadThereAsBefore(t *testing.T) {
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "cloud")
	before, err := Open(root, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := before.SignUp(ctx, Signup{Email: "ada@acme.com", OrgName: "Acme", Slug: "acme"}); err != nil {
		t.Fatal(err)
	}
	before.Close()

	postgres := Settings{DatabaseURL: dbkittest.PostgresURL(t)}
	copied, err := CopyFrom(ctx, filepath.Join(root, databaseFile), postgres)
	if err != nil {
		t.Fatal(err)
	}
	if copied["orgs"] != 1 || copied["people"] != 1 || copied["memberships"] != 1 {
		t.Errorf("copied %v, want the organization, its person and their membership", copied)
	}
	after, err := Open(root, postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if org, found, err := after.Org(ctx, "acme"); err != nil || !found || org.Name != "Acme" {
		t.Errorf("acme on Postgres is %+v, %t, %v", org, found, err)
	}
	if person, found, err := after.only(ctx, "ada@acme.com"); err != nil || !found || person.Org != "acme" {
		t.Errorf("ada on Postgres is %+v, %t, %v", person, found, err)
	}
	if _, err := CopyFrom(ctx, filepath.Join(root, databaseFile), postgres); !errors.Is(err, dbkit.ErrNotEmpty) {
		t.Errorf("a second copy over the accounts gave %v", err)
	}
}

func TestAccountsAreCopiedIntoPostgresOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cloud")
	s, err := Open(root, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	other := "sqlite://" + filepath.Join(t.TempDir(), "other.db")
	if _, err := CopyFrom(context.Background(), filepath.Join(root, databaseFile), Settings{DatabaseURL: other}); err == nil {
		t.Error("the accounts were copied into SQLite")
	}
}
