package web

import (
	"strings"
	"testing"
)

// Every page with the side list offers the person a choice of theme and accent, and the page applies it before it is drawn.
func TestThePageOffersTheThemeAndTheAccentToChoose(t *testing.T) {
	site := open(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/spend", nil).Body.String()
	for _, want := range []string{`id="look"`, `data-mode="dark"`, `data-accent="blue"`, `data-zoom="1"`, `/static/theme.css`, `/static/appearance.js`, `/static/look.js`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not carry %s", want)
		}
	}
}

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
