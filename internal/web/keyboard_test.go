package web

import (
	"strings"
	"testing"
)

// Safari on iOS pans the page when the keyboard opens, so every page loads the script that sizes the page to what is visible.
func TestEveryPageLoadsTheScriptThatKeepsTheHeaderAboveTheKeyboard(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, path := range []string{"/chat", "/jobs"} {
		page := site.ask(t, "GET", path, nil).Body.String()
		if !strings.Contains(page, "/static/keyboard.js") {
			t.Errorf("%s does not load keyboard.js", path)
		}
	}
}
