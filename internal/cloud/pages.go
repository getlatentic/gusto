package cloud

import (
	"net/http"
	"net/url"
)

// view is what a page is drawn from. Every page uses the fields it names.
type view struct {
	Title string
	// Error is a sentence for the person, and Email and OrgName are what they
	// typed, so a refused form is not typed again.
	Error, Email, OrgName string
	// Sent says a sign-in link was asked for.
	Sent bool
	// Token, Action and Button draw the page that confirms a link.
	Token, Action, Button string
	// The organisation page.
	Org      Org
	Person   Person
	Address  string
	Quota    Quota
	Trial    Quota
	Verified Quota
	Notice   string
}

func (s *Server) render(w http.ResponseWriter, status int, page string, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.pages.ExecuteTemplate(w, page, v); err != nil {
		w.Write([]byte("that page could not be drawn"))
	}
}

// link is the address a mailed link points at.
func (s *Server) link(path, secret string) string {
	return s.URL.JoinPath(path).String() + "?t=" + url.QueryEscape(secret)
}
