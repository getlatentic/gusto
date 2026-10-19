package cloud

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
)

func settingOf(env []string, name string) (string, int) {
	value, count := "", 0
	for _, each := range env {
		if v, found := strings.CutPrefix(each, name+"="); found {
			value, count = v, count+1
		}
	}
	return value, count
}

func environmentFor(t *testing.T, operator []string, slug string) []string {
	t.Helper()
	store := open(t, Settings{})
	c := &Controller{Store: store, Cell: &LocalCell{}, Env: operator, FrontDoor: "https://cloud.test"}
	env, err := c.environment(slug, Runtime{Port: 1, ProxyPort: 2}, c.Cell)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func TestEachOrganizationGetsItsOwnPostgresSchemaInTheOperatorsURL(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		operator := []string{"GUSTO_DATABASE_URL=postgres://gusto:pw@db.internal:5432/usage?sslmode=require&search_path=shared"}
		got := map[string]string{}
		for _, slug := range []string{"acme", "globex"} {
			raw, count := settingOf(environmentFor(t, operator, slug), "GUSTO_DATABASE_URL")
			if count != 1 {
				t.Fatalf("%s: the setting is given %d times", slug, count)
			}
			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Host != "db.internal:5432" || parsed.Path != "/usage" || parsed.User.String() != "gusto:pw" ||
				parsed.Query().Get("sslmode") != "require" || len(parsed.Query()["search_path"]) != 1 {
				t.Errorf("%s was given %q", slug, raw)
			}
			got[slug] = parsed.Query().Get("search_path")
		}
		if got["acme"] != "org_acme" || got["globex"] != "org_globex" {
			t.Errorf("schemas %v", got)
		}
	})
}

func TestAHostileSlugBecomesASafeSchemaThatNoOtherSlugShares(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		hostile := `Acme"; DROP SCHEMA public CASCADE; --`
		schema := usageSchema(hostile)
		if len(schema) > 63 || !strings.HasPrefix(schema, "org_") || strings.Trim(schema, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			t.Fatalf("schema %q", schema)
		}
		seen := map[string]string{}
		for _, slug := range []string{"a-b", "a_b", "a--b", "a-b-", hostile, strings.Repeat("x", 80), strings.Repeat("x", 81), strings.Repeat("x", 59), strings.Repeat("x", 60), "acme"} {
			name := usageSchema(slug)
			if len(name) > 63 {
				t.Errorf("%q is %d bytes", name, len(name))
			}
			if other, taken := seen[name]; taken {
				t.Errorf("%q and %q share %q", slug, other, name)
			}
			seen[name] = slug
		}
		if usageSchema("acme") != "org_acme" {
			t.Errorf("a plain slug became %q", usageSchema("acme"))
		}
		raw, _ := settingOf(environmentFor(t, []string{"GUSTO_DATABASE_URL=postgresql://u@h/db"}, hostile), "GUSTO_DATABASE_URL")
		if !strings.Contains(raw, "search_path="+schema) {
			t.Errorf("the hostile slug was given %q", raw)
		}
	})
}

func TestAnythingButPostgresGivesEachServerAnIndexOfItsOwnOnTheLocalDisk(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		index := filepath.Join(t.TempDir(), "local", "index")
		for _, operator := range [][]string{nil, {"GUSTO_DATABASE_URL="}, {"GUSTO_DATABASE_URL=sqlite:///shared/usage.db"}} {
			c := &Controller{Store: open(t, Settings{}), Cell: &LocalCell{}, Env: operator, FrontDoor: "https://cloud.test", IndexDir: index}
			got := map[string]string{}
			for _, slug := range []string{"acme", "globex"} {
				env, err := c.environment(slug, Runtime{Port: 1, ProxyPort: 2}, c.Cell)
				if err != nil {
					t.Fatal(err)
				}
				value, count := settingOf(env, "GUSTO_DATABASE_URL")
				if count != 1 {
					t.Fatalf("%v: the setting is given %d times", operator, count)
				}
				got[slug] = value
				if jobs, _ := settingOf(env, "GUSTO_JOBS"); !strings.Contains(jobs, slug) {
					t.Errorf("the tasks directory is not the organization's: %q", jobs)
				}
			}
			if got["acme"] == got["globex"] {
				t.Errorf("two organizations share an index: %q", got["acme"])
			}
			for slug, raw := range got {
				db, err := dbkit.Open(context.Background(), raw, "/not/used", "usage.db")
				if err != nil {
					t.Fatalf("%v: the server cannot open %q: %v", operator, raw, err)
				}
				db.Close()
				if _, err := os.Stat(filepath.Join(index, slug, "index.db")); err != nil {
					t.Errorf("%v: the index of %s is not under the index directory: %v", operator, slug, err)
				}
			}
		}
	})
}

func TestTheIndexDirectoryIsTheStoresRootByDefaultAndIsNotInAnOrganizationsDataDirectory(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		store := open(t, Settings{})
		c := &Controller{Store: store, Cell: &LocalCell{}, FrontDoor: "https://cloud.test"}
		env, err := c.environment("acme", Runtime{Port: 1, ProxyPort: 2}, c.Cell)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := settingOf(env, "GUSTO_DATABASE_URL")
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "sqlite" || parsed.Path != filepath.ToSlash(filepath.Join(store.Root(), "index", "acme", "index.db")) {
			t.Errorf("%q %v", raw, err)
		}
		if strings.HasPrefix(parsed.Path, filepath.ToSlash(store.DataDir("acme"))) {
			t.Errorf("the index is in the data directory, which hosts may share: %q", raw)
		}
	})
}

func TestAnIndexPathWithASpaceIsOpenedWhereItIs(t *testing.T) {
	index := filepath.Join(t.TempDir(), "my index")
	raw, err := usageDatabase(nil, "acme", index)
	if err != nil {
		t.Fatal(err)
	}
	db, err := dbkit.Open(context.Background(), raw, "/not/used", "usage.db")
	if err != nil {
		t.Fatalf("%q: %v", raw, err)
	}
	db.Close()
	if _, err := os.Stat(filepath.Join(index, "acme", "index.db")); err != nil {
		t.Error(err)
	}
}

func TestADatabaseURLThatDoesNotParseStopsTheServerAndIsNotQuoted(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		c := &Controller{Store: open(t, Settings{}), Cell: &LocalCell{}, Env: []string{"GUSTO_DATABASE_URL=postgres://u:hunter2@h:notaport/db"}}
		_, err := c.environment("acme", Runtime{}, c.Cell)
		if err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("%v", err)
		}
	})
}
