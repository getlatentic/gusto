package desk

import (
	"strings"
	"testing"
)

func TestAMissingTrayIsExplainedBySessionAndDesktop(t *testing.T) {
	cases := []struct {
		name            string
		bus, watcher    bool
		desktop, expect string
	}{
		{"no session", false, false, "", "no D-Bus session"},
		{"no session on gnome", false, false, "GNOME", "no D-Bus session"},
		{"gnome with nothing to show icons", true, false, "ubuntu:GNOME", "AppIndicator extension"},
		{"plain gnome", true, false, "GNOME", "AppIndicator extension"},
		{"another desktop with nothing", true, false, "XFCE", "(XFCE)"},
		{"an unnamed desktop with nothing", true, false, "", "(unknown)"},
		{"kde with a watcher", true, true, "KDE", ""},
		{"gnome with the extension", true, true, "ubuntu:GNOME", ""},
	}
	for _, c := range cases {
		got := explain(c.bus, c.watcher, c.desktop)
		if (c.expect == "") != (got == "") || !strings.Contains(got, c.expect) {
			t.Errorf("%s: %q, want something with %q", c.name, got, c.expect)
		}
		if got != "" && !strings.Contains(got, "runs without") {
			t.Errorf("%s: the helper is not said to go on: %q", c.name, got)
		}
	}
}
