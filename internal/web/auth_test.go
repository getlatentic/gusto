package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A name with dots in it is the ordinary case once a provider signs people in,
// and the signature is separated from the value by a dot. Taking the first one
// cuts the address in half, and every cookie fails a check against itself.
func TestSomebodySignedInByAddressIsStillKnownOnTheNextRequest(t *testing.T) {
	keeper, err := newKeeper("shared")
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"tosinamuda@gmail.com", "the shared sign-in", "a.b.c@sub.example.co.uk"} {
		t.Run(who, func(t *testing.T) {
			issued := httptest.NewRecorder()
			keeper.issueFor(issued, httptest.NewRequest("GET", "/auth/callback", nil), who, Runner)

			next := httptest.NewRequest("GET", "/", nil)
			for _, cookie := range issued.Result().Cookies() {
				next.AddCookie(cookie)
			}
			if !keeper.knows(next) {
				t.Error("the server did not know somebody it had just signed in")
			}
			if got := keeper.who(next); got != who {
				t.Errorf("the server called them %q", got)
			}
			if got := keeper.role(next); got != Runner {
				t.Errorf("the server gave them the role %q", got)
			}
		})
	}
}

// The signature is what the name rests on, so a name somebody edits has to
// stop being known.
func TestAnEditedCookieIsNotKnown(t *testing.T) {
	keeper, err := newKeeper("shared")
	if err != nil {
		t.Fatal(err)
	}
	issued := httptest.NewRecorder()
	keeper.issueFor(issued, httptest.NewRequest("GET", "/auth/callback", nil), "tosinamuda@gmail.com", Runner)
	signed := issued.Result().Cookies()[0]

	for _, edited := range []string{
		signed.Value + "x",
		"9999999999|somebodyelse@gmail.com." + signed.Value[len(signed.Value)-43:],
		"9999999999|somebodyelse@gmail.com",
		// A runner who rewrites their own role to admin keeps the old signature.
		strings.Replace(signed.Value, "|runner|", "|admin|", 1),
		"9999999999|admin|somebodyelse@gmail.com",
		"",
	} {
		next := httptest.NewRequest("GET", "/", nil)
		next.AddCookie(&http.Cookie{Name: cookieName, Value: edited})
		if keeper.knows(next) {
			t.Errorf("%q was taken for a signature this server made", edited)
		}
	}
}
