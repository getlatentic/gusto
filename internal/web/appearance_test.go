package web

import (
	"strings"
	"testing"
)

// The script that sets the theme before a page is drawn is one file, which the React app's pages load as the templates do.
func TestTheThemeIsSetByOneScriptEveryPageLoads(t *testing.T) {
	look, err := assets.ReadFile("static/look.js")
	if err != nil || !strings.Contains(string(look), "window.gustoLook") {
		t.Fatalf("static/look.js does not give window.gustoLook: %v", err)
	}
	if head := appHead(""); !strings.Contains(head, "/static/look.js?v=") || !strings.Contains(head, "/static/theme.css?v=") {
		t.Errorf("the app's pages do not load the theme: %s", head)
	}
}
