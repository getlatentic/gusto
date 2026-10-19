package cli

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/helper"
)

func TestTheStatusSaysWhetherTheIconIsShownAndWhyNotWhenItIsNot(t *testing.T) {
	signed := helper.Status{Server: "https://s", SignedIn: true, Person: "ada", Connected: true}
	if got := statusWords(signed); got != "running, signed in as ada, and following https://s\n" {
		t.Errorf("a helper that says nothing of its icon: %q", got)
	}
	signed.Tray = "shown"
	if got := statusWords(signed); !strings.HasSuffix(got, "icon: shown\n") {
		t.Errorf("%q", got)
	}
	signed.Tray = "none: GNOME does not show tray icons of its own: turn on the AppIndicator extension"
	if got := statusWords(signed); !strings.Contains(got, "icon: none: GNOME") || strings.Count(got, "\n") != 2 {
		t.Errorf("%q", got)
	}
	out := helper.Status{Server: "https://s"}
	if got := statusWords(out); !strings.HasPrefix(got, "running, following https://s, but not signed in") {
		t.Errorf("%q", got)
	}
}
