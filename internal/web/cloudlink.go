package web

import (
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
)

// Cloud makes this server one organization's in a hosted gusto. Nobody signs
// in here: gusto cloud signs a person in, and hands their browser a token
// that this server checks against a key only the two of them hold. An address
// here that wants a sign-in sends the person to gusto cloud.
type Cloud struct {
	// Key is what tokens are made and checked with, Org is the address of the
	// organization this server is and Name what it is called, and URL is where
	// gusto cloud is.
	Key  []byte
	Org  string
	Name string
	URL  string
}

type cloudLink struct {
	Cloud
	seen *entry.Seen
}

// sessionLife is how long what gusto cloud vouched for is trusted, and the
// most a person lingers here after they are removed from the organization. Past
// it they go back through gusto cloud, which asks whether they still belong.
const sessionLife = time.Hour

// UseCloud puts this server behind gusto cloud.
func (s *Server) UseCloud(f Cloud) {
	s.front = &cloudLink{Cloud: f, seen: &entry.Seen{}}
	s.keeper.life = sessionLife
}

// page is the address of a page of gusto cloud, for this organization: a person who is in several
// organizations comes back to the one they left.
func (f *cloudLink) page(path string, query url.Values) string {
	if query == nil {
		query = url.Values{}
	}
	query.Set("org", f.Org)
	return f.URL + path + "?" + query.Encode()
}

// exit is where a person who signed out here goes next: gusto cloud, with a token that asks it to sign them
// out too, or else they would be let straight back in.
func (f *cloudLink) exit(email string) string {
	token, err := entry.MintExit(f.Key, f.Org, email, time.Now())
	if err != nil {
		slog.Error("a sign-out could not be passed to gusto cloud", "error", err)
		return f.page("/org", nil)
	}
	return f.page("/signout", url.Values{"t": {token}})
}

// toCloud sends somebody to be signed in where people are.
func (s *Server) toCloud(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("desktop") {
		s.render(w, "login.html", map[string]any{"Desktop": true, "Provider": true, "Refused": s.refusal(r)})
		return
	}
	http.Redirect(w, r, s.front.page("/open", nil), http.StatusSeeOther)
}

// enter signs a person in with a token gusto cloud made for them. A token that
// is wrong, old, for another organization or already used gets a page that says
// so and links to gusto cloud, and never a redirect there, which two
// servers that disagreed about a key would follow for ever.
func (s *Server) enter(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	claims, err := entry.Verify(s.front.Key, s.front.Org, r.URL.Query().Get("t"), now)
	if err != nil || !s.front.seen.Use(claims, now) {
		s.noteRefused(r, "", "gusto cloud", "the sign-in link was wrong, old or used")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>gusto</title><p>That sign-in link has expired or was used already. <a href="` +
			html.EscapeString(s.front.page("/org", nil)) + `">Open your organization again.</a></p>`))
		return
	}
	role := Runner
	if claims.Role == string(Admin) {
		role = Admin
	}
	s.keeper.issueFor(w, r, claims.Email, role)
	s.keepWorkspace(w, r, claims.Email, r.URL.Query().Get("workspace"))
	s.noteSignIn(r, claims.Email, role, "gusto cloud")
	s.afterSignIn(w, r)
}

// leaseExtend is where a person extends the host this organization runs on, or nothing
// when the server is not behind gusto cloud. The page asks the host, and the host says
// whether it has a lease, so the script is harmless on one that has none.
func (s *Server) leaseExtend() string {
	if s.front == nil {
		return ""
	}
	return s.front.page("/lease", nil)
}
