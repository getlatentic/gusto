package web

import (
	"strings"
	"testing"
)

// A page served under a path loads its built files and its static files from under that path, and a page at the root from
// the root.
func TestAPageUnderAPathLoadsItsFilesFromUnderIt(t *testing.T) {
	page := []byte(`<script type="module" src="/app/assets/index-1.js"></script><link rel="modulepreload" href="/app/assets/react-2.js">` +
		`<link rel="stylesheet" href="/static/tokens.css?v=3"><a href="/chat">chat</a>`)
	got := string(underBase(page, "/workspace/acme-com"))
	for _, want := range []string{`src="/workspace/acme-com/app/assets/index-1.js"`, `href="/workspace/acme-com/app/assets/react-2.js"`,
		`href="/workspace/acme-com/static/tokens.css?v=3"`, `href="/chat"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s is not in %s", want, got)
		}
	}
	if string(underBase(page, "")) != string(page) {
		t.Error("a page at the root was changed")
	}
}
