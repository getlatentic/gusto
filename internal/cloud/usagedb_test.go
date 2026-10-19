package cloud

import (
	"net/url"
	"strings"
	"testing"
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

func TestEachOrganisationGetsItsOwnPostgresSchemaInTheOperatorsURL(t *testing.T) {
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
}

func TestAHostileSlugBecomesASafeSchemaThatNoOtherSlugShares(t *testing.T) {
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
}

func TestAnythingButPostgresIsLeftToEachServersOwnFile(t *testing.T) {
	for _, operator := range [][]string{nil, {"GUSTO_DATABASE_URL="}, {"GUSTO_DATABASE_URL=sqlite:///shared/usage.db"}} {
		env := environmentFor(t, operator, "acme")
		if value, count := settingOf(env, "GUSTO_DATABASE_URL"); count != 0 {
			t.Errorf("%v: given %q", operator, value)
		}
		if jobs, _ := settingOf(env, "GUSTO_JOBS"); !strings.Contains(jobs, "acme") {
			t.Errorf("the tasks directory, which the index sits beside, is not the organisation's: %q", jobs)
		}
	}
}

func TestADatabaseURLThatDoesNotParseStopsTheServerAndIsNotQuoted(t *testing.T) {
	c := &Controller{Store: open(t, Settings{}), Cell: &LocalCell{}, Env: []string{"GUSTO_DATABASE_URL=postgres://u:hunter2@h:notaport/db"}}
	_, err := c.environment("acme", Runtime{}, c.Cell)
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("%v", err)
	}
}
