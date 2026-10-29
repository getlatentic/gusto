package web

import "testing"

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
		if got := personName(who); got != want {
			t.Errorf("%s: %q, want %q", who, got, want)
		}
	}
	if got := initials("ada.lovelace@acme.com"); got != "AL" {
		t.Errorf("%q", got)
	}
}
