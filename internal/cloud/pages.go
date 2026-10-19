package cloud

import (
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/getlatentic/gusto/internal/quota"
)

// view is what a page is drawn from. Every page uses the fields it names.
type view struct {
	Title string
	// Index says a search engine may list the page, which only the landing page and the demo form are. Description, Canonical and Share
	// are what it is told of it and what a link to it shows, and LD is data about it for a search engine.
	Index                         bool
	Description, Canonical, Share string
	LD                            template.JS
	// Lease is set when the host has one, so a page carries the script that warns of its end.
	Lease bool
	// Left and Step draw the lease page.
	Left Left
	Step time.Duration
	// Error is a sentence for the person, and Email and OrgName are what they
	// typed, so a refused form is not typed again.
	Error, Email, OrgName string
	// More says the form also asks for the organization's name and address: a
	// person is asked when the domain could not name it, and keeps being asked
	// after they have been.
	More bool
	Slug string
	// Sent says a sign-in link was asked for.
	Sent bool
	// Token, Action and Button draw the page that confirms a link.
	Token, Action, Button string
	// The organization page.
	Org      Org
	Person   Person
	Address  string
	Quota    quota.Quota
	Trial    quota.Quota
	Verified quota.Quota
	Notice   string
	// Computers is how many computers the organization has.
	Computers int
	// Domain is the company's.
	Domain string
	// The organization's server: how it is, whether a person can open it, and
	// whether the page should look again in a moment.
	Server  string
	CanOpen bool
	Refresh bool
	// Steps and Waited say how far a demo is from ready.
	Steps  []Step
	Waited string
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
	// A demo: the time it has, the state of its server in words, where a call is booked, and whether it may be extended.
	DemoLeft   time.Duration
	DemoWords  string
	Book       string
	Extendable bool
	Ended      bool
	// Ending is a demo that has been stopped and is being deleted.
	Ending bool
	// Video is the landing page's video, and VideoFile says it is a file and not a page to embed. Shot is the picture shown when there is no video.
	Video string
	Shot  string
	// TurnstileSite is the public key of the bot check, which the demo form carries when there is one.
	TurnstileSite string
	VideoFile     bool
	// Name is who a demo is for.
	Name string
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
