package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The kit is the one place every shared piece is seen, so a piece added to ui.html without a place on the kit page fails here.
func TestTheUIKitShowsEveryPiece(t *testing.T) {
	source, err := os.ReadFile("templates/ui.html")
	if err != nil {
		t.Fatal(err)
	}
	kit, err := os.ReadFile("templates/admin-ui-kit.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, piece := range regexp.MustCompile(`\{\{define "(ui-[a-z-]+)"`).FindAllStringSubmatch(string(source), -1) {
		if !strings.Contains(string(kit), `template "`+piece[1]+`"`) {
			t.Errorf("the kit page does not show %s", piece[1])
		}
	}
	h := open(t)
	h.signIn(t)
	if page := h.ask(t, "GET", "/admin/ui-kit", nil); page.Code != 200 {
		t.Fatalf("the kit answered %d", page.Code)
	}
}
