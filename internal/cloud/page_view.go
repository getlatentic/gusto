package cloud

import (
	"github.com/getlatentic/gusto/internal/quota"
)

// pageView is what a page of the web app's cloud entry is drawn from, as JSON inside the page. It holds only what the pages show,
// so nothing a person should not read (an invitation link's digest, a demo's lead) leaves the server, and lengths of time are in
// the words the pages say them in.
type pageView struct {
	Page  string
	Title string

	Error, Ask, Email, OrgName, Slug, Name, Notice string
	More, Taken, WorkOnly, SignedIn, Sent, Google  bool

	Token, Action, Button, Inviter string

	Org      *orgView
	Orgs     []orgView
	Several  bool
	Person   personView
	Address  string
	Domain   string
	Credit   *credit
	Plan     *plan
	Quota    quotaView
	Verified quotaView

	Computers int
	Server    string
	CanOpen   bool

	Members    []memberView
	Invited    []string
	Admins     int
	IsAdmin    bool
	CanInvite  bool
	Workspaces []spaceView
	Link       string

	DemoLeft, DemoLeftLong         string
	DemoWords                      string
	Steps                          []Step
	Book, BookFrame, TurnstileSite string
	Extendable, Ended, Ending      bool

	Lease *leaseView
}

type orgView struct {
	Slug, Name, Admin string
	Demo              bool
}

type personView struct {
	Email string
	Trial bool
}

type quotaView struct {
	Tasks, Concurrent, Members, Computers int
	Monthly                               bool
}

type memberView struct {
	Email, Role, Computer string
}

type spaceView struct {
	ID, Name string
	People   []spacePerson
	Others   []string
}

// leaseView is the host's lease, for the page that keeps it: how long it has left and when it ends, and by how much it is kept.
type leaseView struct {
	Left, Ends, Step string
	CanExtend        bool
}

func orgOf(org Org) orgView {
	return orgView{Slug: org.Slug, Name: org.Name, Admin: org.Admin, Demo: org.Demo != nil}
}

func quotaOf(q quota.Quota) quotaView {
	return quotaView{Tasks: q.Tasks, Concurrent: q.Concurrent, Members: q.Members, Computers: q.Computers, Monthly: q.Monthly}
}

// pageOf is the view a template was given, as the page that replaces the template reads it.
func pageOf(name string, v view) pageView {
	page := pageView{
		Page: name, Title: v.Title,
		Error: v.Error, Ask: v.Ask, Email: v.Email, OrgName: v.OrgName, Slug: v.Slug, Name: v.Name, Notice: v.Notice,
		More: v.More, Taken: v.Taken, WorkOnly: v.WorkOnly, SignedIn: v.SignedIn, Sent: v.Sent, Google: v.Google,
		Token: v.Token, Action: v.Action, Button: v.Button, Inviter: v.Inviter,
		Several: v.Several, Person: personView{Email: v.Person.Email, Trial: v.Person.Trial()},
		Address: v.Address, Domain: v.Domain, Credit: v.Credit, Plan: v.Plan,
		Quota: quotaOf(v.Quota), Verified: quotaOf(v.Verified),
		Computers: v.Computers, Server: v.Server, CanOpen: v.CanOpen,
		Admins: v.Admins, IsAdmin: v.IsAdmin, CanInvite: v.CanInvite, Link: v.Link,
		DemoWords: v.DemoWords, Steps: v.Steps, Book: v.Book, BookFrame: v.BookFrame, TurnstileSite: v.TurnstileSite,
		Extendable: v.Extendable, Ended: v.Ended, Ending: v.Ending,
	}
	page.addPeople(v)
	if v.DemoLeft > 0 {
		page.DemoLeft, page.DemoLeftLong = minutes(v.DemoLeft), minuteLong(v.DemoLeft)
	}
	if name == "lease" {
		page.Lease = leaseOf(v)
	}
	return page
}

// addPeople is the organizations, people, invitations and workspaces a page shows.
func (page *pageView) addPeople(v view) {
	if v.Org.Slug != "" || v.Org.Name != "" {
		org := orgOf(v.Org)
		page.Org = &org
	}
	for _, org := range v.Orgs {
		page.Orgs = append(page.Orgs, orgOf(org))
	}
	for _, member := range v.Members {
		page.Members = append(page.Members, memberView{Email: member.Email, Role: string(member.Role), Computer: v.ComputerOf[member.Email]})
	}
	for _, invited := range v.Invited {
		page.Invited = append(page.Invited, invited.Email)
	}
	for _, row := range v.Workspaces {
		page.Workspaces = append(page.Workspaces, spaceView{ID: row.ID, Name: row.Name, People: row.People, Others: row.Others})
	}
}

func leaseOf(v view) *leaseView {
	lease := &leaseView{Ends: clock(v.Left.Ends), Step: minutes(v.Step), CanExtend: v.Left.CanExtend}
	if v.Left.Remaining > 0 {
		lease.Left = minutes(v.Left.Remaining)
	}
	return lease
}
