package web

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestALocalSignInLinkWorksOnceWithinMinutesAndOnlyWithThisComputersSecret(t *testing.T) {
	l := &localSignIn{secret: []byte("the-secret"), who: "ada", used: map[string]int64{}}
	now := time.Now()
	parts := func(link string) (string, string, string) {
		parsed, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		q := parsed.Query()
		return q.Get("n"), q.Get("e"), q.Get("c")
	}
	n, e, c := parts(LocalLink("the-secret", "0123456789abcdef0123", now))
	if !l.admits(n, e, c, now) {
		t.Fatal("a fresh link made with the secret was refused")
	}
	if l.admits(n, e, c, now) {
		t.Error("a link was followed twice")
	}
	n, e, c = parts(LocalLink("another-secret", "1123456789abcdef0123", now))
	if l.admits(n, e, c, now) {
		t.Error("a link made with another secret was taken")
	}
	n, e, c = parts(LocalLink("the-secret", "2123456789abcdef0123", now))
	if l.admits(n, e, c, now.Add(linkLife+time.Second)) {
		t.Error("a link was taken after it ran out")
	}
	n, e, c = parts(LocalLink("the-secret", "3123456789abcdef0123", now.Add(time.Hour)))
	if l.admits(n, e, c, now) {
		t.Error("a link that runs out far in the future was taken")
	}
	if l.admits("short", e, c, now) || !strings.HasPrefix(LocalLink("s", "4123456789abcdef0123", now), "/local/enter?") {
		t.Error("a short nonce was taken, or the link is not the server's path")
	}
}

func TestThereIsNoLinkSignInWhereTheServerIsNotSomebodysOwn(t *testing.T) {
	site := open(t)
	if got := site.ask(t, "GET", "/local/enter?n=0123456789abcdef0123&e=1&c=x", nil); got.Code != 404 {
		t.Errorf("a shared server answered the local sign-in with %d", got.Code)
	}
}
