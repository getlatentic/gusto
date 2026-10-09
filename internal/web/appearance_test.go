package web

import (
	"strings"
	"testing"
)

// Every page with the side list offers the person a choice of theme and accent, and the page applies it before it is drawn.
func TestThePageOffersTheThemeAndTheAccentToChoose(t *testing.T) {
	site := open(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/jobs", nil).Body.String()
	for _, want := range []string{`id="look"`, `data-mode="dark"`, `data-accent="blue"`, `/static/theme.css`, `/static/appearance.js`, `window.haasLook`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not carry %s", want)
		}
	}
}
