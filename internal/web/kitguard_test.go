package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A kitRule finds a control drawn by hand: either an element of a kind (a dialog, a text field) or a pile of utilities that
// draws a component's look (a floating surface, a pill). Whatever it finds must carry one of the component classes named.
type kitRule struct {
	kind    string
	element *regexp.Regexp   // the tag, for a rule about a kind of element; nil for a rule about a look
	unless  *regexp.Regexp   // a tag the element rule leaves alone
	look    []*regexp.Regexp // every one of these is in the class, for a rule about a look
	classes []string         // one of these is in the class
}

func classLook(s ...string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, each := range s {
		out = append(out, regexp.MustCompile(`(^|\s)`+each+`(\s|$)`))
	}
	return out
}

var kitRules = []kitRule{
	{kind: "dialog", element: regexp.MustCompile(`^<dialog\b`), classes: []string{"dialog"}},
	{kind: "menu", element: regexp.MustCompile(`\brole="menu"`), classes: []string{"dropdown"}},
	{kind: "menu item", element: regexp.MustCompile(`\brole="menuitem"`), classes: []string{"dropdown-item", "dropdown-item-danger"}},
	{kind: "table", element: regexp.MustCompile(`^<table\b`), classes: []string{"data-table"}},
	{kind: "text field", element: regexp.MustCompile(`^<input\b`), unless: regexp.MustCompile(`\btype="(?:hidden|submit|button|checkbox|radio)"`), classes: []string{"input", "input-bare", "sr-only"}},
	{kind: "choice", element: regexp.MustCompile(`^<input\b[^>]*type="(?:checkbox|radio)"`), classes: []string{"input", "sr-only"}},
	{kind: "select", element: regexp.MustCompile(`^<select\b`), classes: []string{"select", "select-inline"}},
	{kind: "text area", element: regexp.MustCompile(`^<textarea\b`), classes: []string{"textarea", "input-bare"}},
	{kind: "solid button", look: classLook(`bg-solid`), classes: []string{"btn", "btn-send"}},
	{kind: "tab", look: classLook(`border-b-2`), classes: []string{"tab"}},
	{kind: "card", look: classLook(`rounded-lg`, `border`, `(?:bg-surface|bg-card)`, `p-[3-8]`), classes: []string{"card", "popover", "facts-card", "figure"}},
	{kind: "floating surface", look: classLook(`(?:shadow-2|shadow-3|shadow-menu|shadow-dialog)`), classes: []string{"popover", "dropdown", "dialog"}},
	{kind: "badge", look: classLook(`rounded-full`, `text-xs`, `px-2`), classes: []string{"badge", "badge-ok", "badge-warn", "badge-bad"}},
	{kind: "menu item", look: classLook(`h-9`, `px-(?:2\.5|3)`, `hover:bg-(?:hover|sunken|wash)`, `text-sm`, `rounded-(?:sm|md|lg)`), classes: []string{"dropdown-item", "dropdown-item-danger", "btn", "btn-quiet"}},
}

// kitExceptions are the places that look like a component and are not one, each with why.
var kitExceptions = map[string]string{
	`admin-ui-kit.html:<span class="grid h-12`:          "a swatch of the solid colour",
	`admin-ui-kit.html:<span class="size-16 rounded-md`: "a swatch of each shadow",
	`layout.html:<a class="skip`:                        "the skip link, which floats only while it has the keyboard",
	`guide.js:bg-solid text-on-solid`:                   "the number of the step the tour is on",
}

// Every page and every script draws a shared control with its component class from tailwind/components.css, so no page can
// drift from the others: a menu, dialog, table, field, card, badge, tab or solid button written out in utilities fails here,
// named by its file and what was found.
func TestPagesDrawSharedControlsWithTheKit(t *testing.T) {
	templates, err := filepath.Glob("templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	tag := regexp.MustCompile(`<[a-z][a-z0-9]*\b(?:[^<>{]|\{\{.*?\}\})*>`)
	for _, file := range templates {
		for _, found := range tag.FindAllString(read(t, file), -1) {
			checkKit(t, filepath.Base(file), found, classOf(found), true)
		}
	}
	scripts, err := filepath.Glob("static/*.js")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`"([^"\\]*(?:\\.[^"\\]*)*)"`)
	for _, file := range scripts {
		if strings.HasSuffix(file, ".min.js") {
			continue
		}
		for _, found := range literal.FindAllStringSubmatch(read(t, file), -1) {
			checkKit(t, filepath.Base(file), found[1], found[1], false)
		}
	}
}

func checkKit(t *testing.T, name, found, class string, isTag bool) {
	t.Helper()
	for _, rule := range kitRules {
		if rule.element != nil && (!isTag || !rule.element.MatchString(found) || rule.unless != nil && rule.unless.MatchString(found)) {
			continue
		}
		if rule.element == nil && !allMatch(rule.look, class) {
			continue
		}
		if hasAny(class, rule.classes) || excepted(name, found) {
			continue
		}
		t.Errorf("%s draws a %s by hand; give it the class %s: %.140s", name, rule.kind, strings.Join(rule.classes, " or "), found)
	}
}

func classOf(tag string) string {
	m := regexp.MustCompile(`\bclass="((?:[^"{]|\{\{.*?\}\})*)"`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1]
}

func allMatch(look []*regexp.Regexp, class string) bool {
	for _, each := range look {
		if !each.MatchString(class) {
			return false
		}
	}
	return true
}

func hasAny(class string, names []string) bool {
	for _, field := range strings.FieldsFunc(class, func(r rune) bool { return r == ' ' || r == '"' || r == '}' || r == '{' }) {
		for _, name := range names {
			if field == name {
				return true
			}
		}
	}
	return false
}

func excepted(name, found string) bool {
	for place := range kitExceptions {
		file, start, _ := strings.Cut(place, ":")
		if file == name && strings.HasPrefix(found, start) {
			return true
		}
	}
	return false
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
