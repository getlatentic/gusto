package cloud

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

// The tests that need a database run once on SQLite and, when GUSTO_TEST_POSTGRES_URL is set, once on Postgres in a
// fresh schema. The first store a test opens gets the database dbkittest made, and any other store the test opens
// gets a fresh database of the same kind.
var (
	testDialect dbkit.Dialect
	spareDB     *dbkit.DB
)

func onEachDatabase(t *testing.T, body func(t *testing.T)) {
	t.Helper()
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		testDialect, spareDB = db.Dialect(), db
		body(t)
	})
}

func freshDatabase(t testing.TB) *dbkit.DB {
	t.Helper()
	if db := spareDB; db != nil {
		spareDB = nil
		return db
	}
	if testDialect == dbkit.Postgres {
		return dbkittest.Postgres(t)
	}
	return dbkittest.SQLite(t)
}

func open(t testing.TB, set Settings) *Store {
	t.Helper()
	s, err := openOn(freshDatabase(t), filepath.Join(t.TempDir(), "cloud"), set)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// only is the membership of an address that is in one organization, which is what most tests make.
func (s *Store) only(ctx context.Context, email string) (Person, bool, error) {
	all, err := s.Memberships(ctx, email)
	if err != nil || len(all) != 1 {
		return Person{}, false, err
	}
	return all[0], true, nil
}

// shown is the data a page was drawn from, read from the JSON the server put in it.
func shown(t testing.TB, body string) pageView {
	t.Helper()
	const open, end = `<script id="gusto-data" type="application/json">`, `</script>`
	_, rest, found := strings.Cut(body, open)
	data, _, closed := strings.Cut(rest, end)
	if !found || !closed {
		t.Fatalf("the page carries no data: %.300s", body)
	}
	var page pageView
	if err := json.Unmarshal([]byte(data), &page); err != nil {
		t.Fatalf("the page's data is not a page: %v: %.300s", err, data)
	}
	return page
}

// styled checks that a page links the tokens by their digest and that the stylesheet the web app built for it is served from host
// to be kept, since a new build names another.
func styled(t testing.TB, b *browser, host, body string) {
	t.Helper()
	if !strings.Contains(body, `<link rel="stylesheet" href="/static/tokens.css?v=`) {
		t.Errorf("the page does not link the tokens by their digest: %.600s", body)
	}
	sheet := ""
	for _, link := range strings.Split(body, `href="/app/assets/`)[1:] {
		if name, _, _ := strings.Cut(link, `"`); strings.HasSuffix(name, ".css") {
			sheet = name
		}
	}
	if sheet == "" {
		t.Fatalf("the page links no stylesheet of the web app: %.600s", body)
	}
	status, css, header := b.at(host, "GET", "/app/assets/"+sheet, nil)
	if status != 200 || !strings.Contains(header.Get("Content-Type"), "css") || len(css) == 0 || !strings.Contains(header.Get("Cache-Control"), "immutable") {
		t.Errorf("the page's stylesheet %s: %d %v", sheet, status, header)
	}
}

// shownAt is the data of the page a browser is shown at a path.
func shownAt(t testing.TB, b *browser, path string) pageView {
	t.Helper()
	_, body := b.get(path)
	return shown(t, body)
}
