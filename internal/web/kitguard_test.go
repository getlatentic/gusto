package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// kitRule says which kit piece draws one kind of control: a tag the rule matches must name the piece in its class.
type kitRule struct {
	kind  string
	match *regexp.Regexp
	piece string
}

var kitRules = []kitRule{
	{"dialog", regexp.MustCompile(`^<dialog\b`), `template "ui-dialog"`},
	{"menu", regexp.MustCompile(`\brole="menu"`), `template "ui-menu"`},
	{"menu item", regexp.MustCompile(`\brole="menuitem"`), `template "ui-menu-item`},
	{"table", regexp.MustCompile(`^<table\b`), `template "ui-table"`},
	{"solid button", regexp.MustCompile(`\bbg-solid\b`), `template "ui-btn`},
	{"tab", regexp.MustCompile(`\bborder-b-2\b`), `template "ui-tab"`},
}

// kitExceptions are the tags that look like a kit control and are not one, each with why.
var kitExceptions = map[string]string{
	"file.html:<table class=\"w-auto min-w-full text-xs": "a CSV file shown as its grid of cells, not a table of records",
	"task.html:<table class=\"w-full font-mono":          "a diff, numbered line by line",
	"admin-ui-kit.html:<span class=\"grid h-12":          "a swatch of the solid colour",
}

// Every page draws a shared control with its kit piece in ui.html, so no page can drift from the others: a copy of a menu,
// dialog, table, tab or solid button written out by hand fails here, named by its file and its tag.
func TestPagesDrawSharedControlsWithTheKit(t *testing.T) {
	files, err := filepath.Glob("templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	tag := regexp.MustCompile(`<[a-z][a-z0-9]*\b[^<>]*>`)
	for _, file := range files {
		name := filepath.Base(file)
		if name == "ui.html" {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, found := range tag.FindAllString(string(source), -1) {
			for _, rule := range kitRules {
				if !rule.match.MatchString(found) || strings.Contains(found, rule.piece) || excepted(name, found) {
					continue
				}
				t.Errorf("%s draws a %s by hand; use %s}}: %.120s", name, rule.kind, rule.piece, found)
			}
		}
	}
}

func excepted(name, found string) bool {
	for prefix := range kitExceptions {
		file, start, _ := strings.Cut(prefix, ":")
		if file == name && strings.HasPrefix(found, start) {
			return true
		}
	}
	return false
}
