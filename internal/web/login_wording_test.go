package web

import (
	"testing"
)

// signInOf is what the sign-in page offers at the query given, as web/src/pages/sign-in/types.ts names it.
func signInOf(t *testing.T, s *site, query string) signInView {
	t.Helper()
	var page struct{ SignIn signInView }
	s.data(t, "/login"+query, &page)
	return page.SignIn
}

// A wrong password is answered in plain words, which the page says: see web/src/pages/sign-in/sign-in-page.test.tsx. A server
// with a shared password is not one that opens with a link, so the page does not send a person to a link they do not have.
func TestAWrongPasswordIsAnsweredInPlainWords(t *testing.T) {
	site := open(t)
	page := signInOf(t, site, "?wrong=1")
	if !page.Wrong {
		t.Error("the sign-in page does not say the password is not right")
	}
	if page.Local {
		t.Error("a server with a shared password sends a person to a link they do not have")
	}
}
