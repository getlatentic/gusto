package dbkit

import (
	"errors"
	"strings"
	"testing"
)

func TestLogURLHasNoPassword(t *testing.T) {
	for _, u := range []string{
		"postgres://ada:hunter2@127.0.0.1:1/db?sslmode=disable&search_path=s",
		"postgres://ada@127.0.0.1:1/db?password=hunter2&sslmode=disable&search_path=s",
	} {
		tg, err := parseTarget(u, "", "x.db")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(tg.logURL, "hunter2") || !strings.Contains(tg.logURL, "ada@127.0.0.1:1/db") ||
			!strings.Contains(tg.logURL, "search_path=s") {
			t.Fatalf("logURL(%q) = %q", u, tg.logURL)
		}
	}
}

func TestRedactReplacesEverySecret(t *testing.T) {
	tg, err := parseTarget("postgres://ada:p%40ss@127.0.0.1/db", "", "x.db")
	if err != nil {
		t.Fatal(err)
	}
	got := tg.redact(errors.New("dial postgres://ada:p%40ss@127.0.0.1/db as p@ss"))
	if strings.Contains(got.Error(), "p@ss") || strings.Contains(got.Error(), "p%40ss") {
		t.Fatalf("redact = %q", got)
	}
}

func TestRebindSkipsQuotedQuestionMarks(t *testing.T) {
	got := dollarPlaceholders(`SELECT a FROM t WHERE b = ? AND c = 'what?' AND d = "x?" AND e = ?`)
	want := `SELECT a FROM t WHERE b = $1 AND c = 'what?' AND d = "x?" AND e = $2`
	if got != want {
		t.Fatalf("got %s", got)
	}
}
