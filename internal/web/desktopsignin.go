package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// The desktop app signs a person in through their own browser, where their provider's page, passkeys and sign-in are, and not in a
// window of its own, which a provider may refuse. The app keeps a secret and gives the browser only its hash. After the person has
// signed in as they would for any page, the browser is sent back to the app with a ticket, and the app trades the ticket and its secret
// for a session in its own window. A ticket is good once, for two minutes, and only to whoever holds the secret it was made for, so a
// link another program on the computer saw is no use to it.

const (
	desktopCookie = "haas-desktop"
	// ticketLife is how long the browser has to hand a ticket to the app.
	ticketLife = 2 * time.Minute
	// challengeLength is a SHA-256 hash in unpadded base64url.
	challengeLength = 43
)

type ticket struct {
	who       string
	role      Role
	challenge string
	until     time.Time
}

// tickets are the sign-ins waiting to be taken by an app.
type tickets struct {
	mu   sync.Mutex
	held map[string]ticket
}

func (t *tickets) mint(who string, role Role, challenge string, now time.Time) string {
	id := randomValue()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.held == nil {
		t.held = map[string]ticket{}
	}
	for held, each := range t.held {
		if !now.Before(each.until) {
			delete(t.held, held)
		}
	}
	t.held[id] = ticket{who: who, role: role, challenge: challenge, until: now.Add(ticketLife)}
	return id
}

// redeem spends a ticket. It is spent whether or not the secret is the right one, so a ticket cannot be guessed at.
func (t *tickets) redeem(id, secret string, now time.Time) (who string, role Role, ok bool) {
	t.mu.Lock()
	held, found := t.held[id]
	delete(t.held, id)
	t.mu.Unlock()
	if !found || !now.Before(held.until) {
		return "", "", false
	}
	sum := sha256.Sum256([]byte(secret))
	made := base64.RawURLEncoding.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(made), []byte(held.challenge)) != 1 {
		return "", "", false
	}
	return held.who, held.role, true
}

func validChallenge(value string) bool {
	if len(value) != challengeLength {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil
}

// desktopStart is where the app sends the browser. It remembers whose hash this is, and sends the person to sign in when they have
// not, which brings them back through afterSignIn.
func (s *Server) desktopStart(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("challenge")
	if !validChallenge(challenge) {
		http.Error(w, "that is not a sign-in the app started", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: desktopCookie, Value: challenge, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: overTLS(r), Expires: time.Now().Add(10 * time.Minute),
	})
	if s.keeper.knows(r) {
		http.Redirect(w, r, "/auth/desktop/handoff", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// afterSignIn is where somebody goes once they are signed in: the page they would see, or back to the app that sent them.
func (s *Server) afterSignIn(w http.ResponseWriter, r *http.Request) {
	if validChallenge(cookieValue(r, desktopCookie)) {
		http.Redirect(w, r, "/auth/desktop/handoff", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// desktopHandoff gives the signed-in browser a ticket and sends it to the app.
func (s *Server) desktopHandoff(w http.ResponseWriter, r *http.Request) {
	challenge := cookieValue(r, desktopCookie)
	http.SetCookie(w, &http.Cookie{Name: desktopCookie, Value: "", Path: "/", MaxAge: -1})
	if !validChallenge(challenge) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	who, role, _ := s.keeper.session(r)
	id := s.tickets.mint(who, role, challenge, time.Now())
	link := "latentic://signed-in?ticket=" + url.QueryEscape(id)
	s.render(w, "desktop-handoff.html", map[string]any{"Link": template.URL(link)})
}

// desktopRedeem signs the app's own window in, for a ticket and the secret whose hash the ticket was made for. It is answered only
// to a request the app made itself: a page on another site that sent somebody here would sign them in as somebody else.
func (s *Server) desktopRedeem(w http.ResponseWriter, r *http.Request) {
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
		http.Error(w, "that sign-in is for the app", http.StatusForbidden)
		return
	}
	query := r.URL.Query()
	who, role, ok := s.tickets.redeem(query.Get("ticket"), query.Get("secret"), time.Now())
	if !ok {
		s.noteRefused(r, "", "the desktop app", "the sign-in ticket was wrong, old or used")
		refuse(w, r, "that sign-in expired; try again")
		return
	}
	s.keeper.issueFor(w, r, who, role)
	s.noteSignIn(r, who, role, "the desktop app")
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}
