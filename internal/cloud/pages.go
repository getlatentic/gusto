package cloud

import (
	"net/http"
	"net/url"
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

// view is what a page is drawn from. Every page uses the fields it names.
type view struct {
	Title string
	// Lease is set when the host has one, so a page carries the script that warns of its end.
	Lease bool
	// Left and Step draw the lease page.
	Left Left
	Step time.Duration
	// Error is a sentence for the person, and Email and OrgName are what they
	// typed, so a refused form is not typed again.
	Error, Email, OrgName string
	// More says the form also asks for the organisation's name and address: a
	// person is asked when the domain could not name it, and keeps being asked
	// after they have been.
	More bool
	Slug string
	// Sent says a sign-in link was asked for.
	Sent bool
	// Token, Action and Button draw the page that confirms a link.
	Token, Action, Button string
	// The organisation page.
	Org      Org
	Person   Person
	Address  string
	Quota    quota.Quota
	Trial    quota.Quota
	Verified quota.Quota
	Notice   string
	// Computers is how many computers the organisation has.
	Computers int
	// Domain is the company's.
	Domain string
	// The organisation's server: how it is, whether a person can open it, and
	// whether the page should look again in a moment.
	Server  string
	CanOpen bool
	Refresh bool
	// The people: who is in, who has been invited and has not answered, whether the
	// person looking is an admin, and whether they may invite one more.
	Members   []Person
	Invited   []Invitation
	IsAdmin   bool
	CanInvite bool
	// Admins is how many of the members are admins, since the last one cannot be made
	// a runner and is not offered it.
	Admins int
	// Computers of each member by address, for the people who have one.
	ComputerOf map[string]string
	Inviter    string
}

func (s *Server) render(w http.ResponseWriter, status int, page string, v view) {
	v.Lease = s.Lease != nil
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
