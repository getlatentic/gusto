package cloud

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/getlatentic/gusto/internal/web"

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
	// More says the form also asks for the organization's name: a person is asked when their company has an organization
	// already, and keeps being asked after they have been.
	More bool
	// Ask is the question the form asks next when nothing was wrong, only missing, so it is not shown as a refusal.
	Ask string
	// Taken says the address a person signed up with has an account, so they are offered a sign-in link to it.
	Taken bool
	// WorkOnly says only a work address may sign up or start a demo.
	WorkOnly bool
	// ForTeam says the demo form is the team's, where only the team starts demos, for a call with someone else.
	ForTeam bool
	// Google says a person may sign in and sign up with a Google account.
	Google bool
	// SignedIn says the person looking at the landing page is signed in, so it offers to open gusto rather than to sign in.
	SignedIn bool
	// Sent says a sign-in link was asked for.
	Sent bool
	// Token, Action and Button draw the page that confirms a link.
	Token, Action, Button string
	// The organization page, and Several when the person looking is in more organizations than this one.
	Org     Org
	Several bool
	// Credit is what is left of the organization's free credit on the models gusto provides.
	Credit *credit
	// Plan is how the organization pays, when this gusto cloud server bills.
	Plan *plan
	// Orgs are the organizations a person chooses among.
	Orgs     []Org
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
	// RefreshEvery is how many seconds the page waits to look again, three when it is not set.
	RefreshEvery int
	// Steps and Waited say how far a demo is from ready.
	Steps  []Step
	Waited string
	// The people: who is in, who has been invited and has not answered, whether the
	// person looking is an admin, and whether they may invite one more.
	Members []Person
	// Teams is the organization's teams, shown on its page when an admin may change them or there are several.
	Teams     []teamRow
	Invited   []Invitation
	IsAdmin   bool
	CanInvite bool
	// Admins is how many of the members are admins, since the last one cannot be made
	// a runner and is not offered it.
	Admins int
	// Computers of each member by address, for the people who have one.
	ComputerOf map[string]string
	Inviter    string
	// Link is an invitation's address, shown to the person who made it when mail is not the only way it travels.
	Link string
	// A demo: the time it has, the state of its server in words, where a call is booked, and whether it may be extended.
	DemoLeft   time.Duration
	DemoWords  string
	Book       string
	Extendable bool
	Ended      bool
	// BookFrame is the booking page shown in the page, where the booking service may be framed, and Wide gives a page the room
	// for it.
	BookFrame string
	Wide      bool
	// Console draws the page in the app's admin layout instead of a card.
	Console bool
	// Ending is a demo that has been stopped and is being deleted.
	Ending bool
	// TurnstileSite is the public key of the bot check, which the demo form carries when there is one.
	TurnstileSite string
	// Name is who a demo is for.
	Name string
}

// render answers with a page of the web app's cloud entry, drawn from the view as JSON inside it, so a refused form is answered
// with what was typed and no second request.
func (s *Server) render(w http.ResponseWriter, status int, page string, v view) {
	v.Lease = s.Lease != nil
	v.WorkOnly = s.Store != nil && !s.Store.FreeMailAllowed()
	v.Google = s.Google != nil
	shown := pageOf(page, v)
	for i := range shown.Orgs {
		shown.Orgs[i].Address = s.orgShown(shown.Orgs[i].Slug)
	}
	data, err := json.Marshal(shown)
	if err != nil {
		http.Error(w, "that page could not be drawn", http.StatusInternalServerError)
		return
	}
	web.WriteAppPage(w, status, web.CloudEntry, pageHead(v), string(data))
}

// link is the address a mailed link points at.
func (s *Server) link(path, secret string) string {
	return s.URL.JoinPath(path).String() + "?t=" + url.QueryEscape(secret)
}
