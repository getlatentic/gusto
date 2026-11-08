package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/roomlive/livetest"
)

// Behind a proxy that ends TLS, the session cookie has the name a secure connection gives it. A socket that was opened that way
// keeps asking the rules about the cookie under that name, so what it may do is judged as the page's own routes judge it.
func TestASocketOpenedBehindATLSProxyIsJudgedByTheSecureCookie(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	forwarded := httptest.NewRequest("GET", "/auth/callback", nil)
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	issued := httptest.NewRecorder()
	l.site.server.keeper.issueFor(issued, forwarded, alice, Runner)
	var secure *http.Cookie
	for _, cookie := range issued.Result().Cookies() {
		if cookie.Name == secureCookieName {
			secure = cookie
		}
	}
	if secure == nil {
		t.Fatal("a forwarded https request should be given the secure cookie")
	}

	header := http.Header{"Cookie": {secure.Name + "=" + secure.Value}, "X-Forwarded-Proto": {"https"}}
	c, status, err := livetest.Connect(t, l.http.URL, "/live?room="+l.id, header, false)
	if err != nil {
		t.Fatalf("a socket behind a TLS proxy: %v (%d)", err, status)
	}
	defer c.Close()
	if got := c.Subscribe(roomlive.StreamChannel(l.id), nil); got.Err != nil {
		t.Fatalf("subscribing: %s", got.Err.Message)
	}
	if result := l.say(c, "c-1", "hello from behind a proxy", false); !result.OK {
		t.Fatalf("saying it: %+v", result)
	}

	// The same cookie without the forwarded scheme is not a session: the name it would be looked up under differs.
	if _, status, err := livetest.Connect(t, l.http.URL, "/live?room="+l.id, http.Header{"Cookie": {secure.Name + "=" + secure.Value}}, false); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("a secure cookie sent as if over plain http should not sign anyone in: %d %v", status, err)
	}
}
