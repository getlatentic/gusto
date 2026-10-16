package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/changelog"
)

// UseTokens makes the API take these tokens and not the shared password. Without
// them the API takes the shared password, and the pages that manage tokens
// answer 404.
func (s *Server) UseTokens(store *apitoken.Store) { s.tokens = store }

// tokenRow is one token as the page says it.
type tokenRow struct {
	ID       string
	Name     string
	May      string
	Made     string
	MadeDay  string
	By       string
	Ended    string
	LastUsed string
	UsedAt   int64
	Live     bool
}

// tokenScopeWords say what each scope lets a token do, for the person choosing.
var tokenScopeWords = map[string]string{
	apitoken.Submit:  "send jobs",
	apitoken.Spend:   "read spending",
	apitoken.Audit:   "read the change record",
	apitoken.Metrics: "read metrics",
	apitoken.Work:    "act as one person, on their own work",
}

func (s *Server) tokensPage(w http.ResponseWriter, r *http.Request) {
	s.showTokens(w, r, map[string]any{})
}

// showTokens draws the page with what the caller adds: a refusal, or the secret
// of the token just made, which is on no other page and in no other request.
func (s *Server) showTokens(w http.ResponseWriter, r *http.Request, page map[string]any) {
	var rows []tokenRow
	for _, held := range s.tokens.List() {
		rows = append(rows, tokenRow{
			ID: held.ID, Name: held.Name, May: describeScopes(held.Scopes), By: held.By, Live: held.Live(),
			Made: shownTime(held.Made), MadeDay: shortDay(held.Made), Ended: endedOf(held), LastUsed: shownTime(held.LastUsed), UsedAt: held.LastUsed,
		})
	}
	var offered []map[string]string
	for _, scope := range apitoken.Scopes {
		offered = append(offered, map[string]string{"Scope": scope, "Words": tokenScopeWords[scope]})
	}
	page["Title"] = productName + " — API tokens"
	page["Tokens"], page["Offered"] = rows, offered
	w.Header().Set("Cache-Control", "no-store")
	s.drawConsole(w, "admin-tokens.html", "tokens", page)
}

func describeScopes(scopes []string) string {
	words := make([]string, len(scopes))
	for i, scope := range scopes {
		words[i] = tokenScopeWords[scope]
	}
	return strings.Join(words, ", ")
}

func shownTime(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format("2 Jan 2006 15:04 UTC")
}

// shortDay is the day something happened, with the year only when it is not this one.
func shortDay(unix int64) string {
	if unix == 0 {
		return ""
	}
	when := time.Unix(unix, 0).UTC()
	if when.Year() == time.Now().UTC().Year() {
		return when.Format("2 Jan")
	}
	return when.Format("2 Jan 2006")
}

func endedOf(held apitoken.Token) string {
	if held.Live() {
		return ""
	}
	return shownTime(held.Revoked) + " by " + or(held.RevokedBy, "nobody signed in")
}

// makeToken makes a token and shows its secret in this one response.
func (s *Server) makeToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.showTokens(w, r, map[string]any{"Refused": err.Error()})
		return
	}
	who := s.keeper.who(r)
	token, secret, err := s.tokens.Create(r.FormValue("name"), r.Form["scope"], who)
	if err != nil {
		s.showTokens(w, r, map[string]any{"Refused": err.Error()})
		return
	}
	s.note(r, changelog.TokenCreated, map[string]string{"id": token.ID, "name": token.Name, "scopes": describeScopes(token.Scopes)})
	s.showTokens(w, r, map[string]any{"Secret": secret, "Made": token.Name})
}

// revokeToken ends a token. It takes effect on the next request the token makes.
func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	held, err := s.tokens.Revoke(r.PathValue("id"), s.keeper.who(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.note(r, changelog.TokenRevoked, map[string]string{"id": held.ID, "name": held.Name})
	http.Redirect(w, r, "/admin/tokens", http.StatusSeeOther)
}
