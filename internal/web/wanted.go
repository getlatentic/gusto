package web

import (
	"net/http"
	"strings"
)

// wantedCookie remembers, through a sign-in, the page a person asked for before they were signed in.
const wantedCookie = "gusto_wanted"

// mostWanted is the longest address that is remembered; a longer one is not, and sign-in lands on the home page.
const mostWanted = 2048

// rememberWanted keeps the page a request asked for, when it is a page of this server that a sign-in can go back to: a GET
// for a path of its own, not one that would send the person elsewhere.
func rememberWanted(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.RequestURI()
	if r.Method != http.MethodGet || !ownPath(wanted) || len(wanted) > mostWanted {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: wantedCookie, Value: wanted, Path: "/", MaxAge: 600, HttpOnly: true,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteLaxMode})
}

// wantedPage is where a person goes once signed in: the page they asked for, when one was remembered, else the home page.
// What was remembered is used once.
func wantedPage(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(wantedCookie)
	if err != nil {
		return "/"
	}
	http.SetCookie(w, &http.Cookie{Name: wantedCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	if !ownPath(cookie.Value) || len(cookie.Value) > mostWanted {
		return "/"
	}
	return cookie.Value
}

// ownPath is a path on this server: it begins with one slash, and nothing a browser would read as another host.
func ownPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "/\\") &&
		!strings.ContainsAny(path, "\\\r\n") && !strings.HasPrefix(path, "/auth/") && !strings.HasPrefix(path, "/login")
}
