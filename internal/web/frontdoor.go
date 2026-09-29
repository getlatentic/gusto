package web

import (
	"html"
	"net/http"
	"time"

	"github.com/getlatentic/haas/internal/entry"
)

// FrontDoor makes this server one organisation's in a hosted haas. Nobody signs
// in here: the front door signs a person in, and hands their browser a token
// that this server checks against a key only the two of them hold. An address
// here that wants a sign-in sends the person to the front door.
type FrontDoor struct {
	// Key is what tokens are made and checked with, Org is the organisation
	// this server is, and URL is where the front door is.
	Key []byte
	Org string
	URL string
}

type frontDoor struct {
	FrontDoor
	seen *entry.Seen
}

// sessionLife is how long what the front door vouched for is trusted, and the
// most a person lingers here after they are removed from the organisation. Past
// it they go back through the front door, which asks whether they still belong.
const sessionLife = time.Hour

// UseFrontDoor puts this server behind a front door.
func (s *Server) UseFrontDoor(f FrontDoor) {
	s.front = &frontDoor{FrontDoor: f, seen: &entry.Seen{}}
	s.keeper.life = sessionLife
}

// toFrontDoor sends somebody to be signed in where people are.
func (s *Server) toFrontDoor(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.front.URL+"/open", http.StatusSeeOther)
}

// enter signs a person in with a token the front door made for them. A token that
// is wrong, old, for another organisation or already used gets a page that says
// so and links to the front door, and never a redirect there, which two
// servers that disagreed about a key would follow for ever.
func (s *Server) enter(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	claims, err := entry.Verify(s.front.Key, s.front.Org, r.URL.Query().Get("t"), now)
	if err != nil || !s.front.seen.Use(claims, now) {
		s.noteRefused(r, "", "the front door", "the sign-in link was wrong, old or used")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>haas</title><p>That sign-in link has expired or was used already. <a href="` +
			html.EscapeString(s.front.URL) + `/org">Open your organisation again.</a></p>`))
		return
	}
	role := Runner
	if claims.Role == string(Admin) {
		role = Admin
	}
	s.keeper.issueFor(w, r, claims.Email, role)
	s.noteSignIn(r, claims.Email, role, "the front door")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
