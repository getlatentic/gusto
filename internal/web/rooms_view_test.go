package web

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
)

func TestARoomCallsAPersonByTheNameInTheirAddress(t *testing.T) {
	for who, want := range map[string]string{
		"ada.lovelace@acme.com":        "Ada Lovelace",
		"tosinamuda+container1029@x.y": "Tosinamuda",
		"bob@example.com":              "Bob",
		"mary-jane_watson@x.y":         "Mary Jane Watson",
		"élodie@x.y":                   "Élodie",
		"+only@x.y":                    "",
		sharedSignIn:                   "Owner",
	} {
		if got := nameFromAddress(who); got != want {
			t.Errorf("%s: %q, want %q", who, got, want)
		}
	}
	if got := initialsOf(nameFromAddress("ada.lovelace@acme.com")); got != "AL" {
		t.Errorf("%q", got)
	}
}

// A person who gave the front door a name is called by it, and by its initials; nobody else's name changes, and a roster
// written again is read again.
func TestAPersonIsCalledByTheNameTheyGaveAndOthersByTheirAddress(t *testing.T) {
	site := open(t)
	path := filepath.Join(t.TempDir(), "members.json")
	write := func(name string) {
		if err := roster.Write(path, roster.Roster{Members: []roster.Member{{Email: "priya@getlatentic.com", Name: name, Role: "admin"}, {Email: "sam.okafor@getlatentic.com", Role: "runner"}}}); err != nil {
			t.Fatal(err)
		}
	}
	write("Priya Nair")
	site.server.UseRoster(path)
	if got := site.server.personName("Priya@getlatentic.com"); got != "Priya Nair" {
		t.Errorf("she is called %q", got)
	}
	if got := site.server.initials("priya@getlatentic.com"); got != "PN" {
		t.Errorf("her initials are %q", got)
	}
	if got := site.server.personName("sam.okafor@getlatentic.com"); got != "Sam Okafor" {
		t.Errorf("a person who gave no name is called %q", got)
	}
	if got := site.server.ownerLabel("priya@getlatentic.com", "sam.okafor@getlatentic.com"); got != "Priya Nair's agent" {
		t.Errorf("her agent is %q", got)
	}
	time.Sleep(nameRecheck + 10*time.Millisecond)
	write("Priya N. Nair")
	if got := site.server.personName("priya@getlatentic.com"); got != "Priya N. Nair" {
		t.Errorf("a roster written again was not read again: %q", got)
	}
}
