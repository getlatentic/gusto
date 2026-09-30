package cloud

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func (s *Server) demoOffered() bool { return s.Store.set.Demo.Enabled() }

// demoForm is the page a visitor starts a demo from.
func (s *Server) demoForm(w http.ResponseWriter, r *http.Request) {
	if !s.demoOffered() {
		http.NotFound(w, r)
		return
	}
	s.render(w, http.StatusOK, "demo", view{Title: "Try haas", Book: s.Store.set.Demo.BookURL, DemoLeft: s.Store.set.Demo.Length})
}

// startDemo makes a demo for a visitor, signs them in to it and sends them to wait for it. A visitor who was refused
// sees why and keeps what they typed.
func (s *Server) startDemo(w http.ResponseWriter, r *http.Request) {
	if !s.demoOffered() {
		http.NotFound(w, r)
		return
	}
	name, email, company := r.PostFormValue("name"), r.PostFormValue("email"), r.PostFormValue("company")
	again := func(status int, why string) {
		s.render(w, status, "demo", view{Title: "Try haas", Error: why, Email: email, OrgName: company, Name: name,
			Book: s.Store.set.Demo.BookURL, DemoLeft: s.Store.set.Demo.Length})
	}
	if !s.all.Allow("demo") {
		again(http.StatusServiceUnavailable, "Demos are busy right now. Try again in a little while, or book a call.")
		return
	}
	if !s.signups.Allow("demo:" + s.source(r)) {
		again(http.StatusTooManyRequests, "That is a lot of demos from one place. Try again later.")
		return
	}
	person, org, err := s.Store.StartDemo(name, email, company)
	if err != nil {
		again(demoStatus(err), demoText(err))
		return
	}
	s.provision(org.Slug)
	s.Sessions.Issue(w, person.Email)
	http.Redirect(w, r, "/demo/wait", http.StatusSeeOther)
}

func demoText(err error) string {
	switch {
	case errors.Is(err, ErrDemoFull):
		return "Every demo is in use right now. Book a call and we will show you."
	case errors.Is(err, ErrEmailTaken):
		return "That address has a demo running. Sign in to get back to it."
	case errors.Is(err, ErrDemoNeedsName), errors.Is(err, ErrNeedCompany):
		return refusalSentence(err)
	}
	for _, known := range []error{ErrBadEmail, ErrFreeMail, ErrDisposable, ErrBadOrgName} {
		if errors.Is(err, known) {
			return refusalSentence(known)
		}
	}
	log.Printf("cloud: a demo could not be started: %v", err)
	return "Something went wrong on our side. Try again."
}

func demoStatus(err error) int {
	switch {
	case errors.Is(err, ErrDemoFull):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrEmailTaken):
		return http.StatusConflict
	case errors.Is(err, ErrDemoNeedsName), errors.Is(err, ErrNeedCompany), errors.Is(err, ErrBadEmail), errors.Is(err, ErrFreeMail), errors.Is(err, ErrDisposable), errors.Is(err, ErrBadOrgName):
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

// demoWait is the page a demo's visitor is on until their server answers, which sends them in once, and is where
// they come back to for the time they have, to extend it, and to book a call.
func (s *Server) demoWait(w http.ResponseWriter, r *http.Request, person Person) {
	demo, left, found := s.Store.DemoLeft(person.Org)
	if !found {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	state := Ready
	if s.Controller != nil {
		_, state = s.Controller.Route(person.Org)
	}
	page := view{Title: "Your haas demo", Org: Org{Slug: person.Org, Name: demo.Company}, Person: person, DemoLeft: left,
		Book: s.Store.set.Demo.BookURL, Extendable: !demo.Extended && s.Store.set.Demo.Extend > 0 && !demo.Expires.IsZero(),
		Notice: r.URL.Query().Get("notice")}
	if state != Ready {
		page.Refresh, page.DemoWords = true, waitingWords(state)
		s.render(w, http.StatusOK, "demo-wait", page)
		return
	}
	if demo.Ready.IsZero() {
		if _, err := s.Store.DemoReady(person.Org); err != nil {
			log.Printf("cloud: a demo's time was not started: %v", err)
		}
		http.Redirect(w, r, "/open", http.StatusSeeOther)
		return
	}
	page.CanOpen = true
	s.render(w, http.StatusOK, "demo-wait", page)
}

// waitingWords says what a visitor is waiting for.
func waitingWords(state State) string {
	if state == Failed {
		return "Something went wrong starting your demo. It is being tried again."
	}
	return "Getting your demo ready. When nothing else is running this takes about two minutes, and your time starts when it is ready, not before."
}

func (s *Server) extendDemo(w http.ResponseWriter, r *http.Request, person Person) {
	_, err := s.Store.ExtendDemo(person.Org)
	notice := ""
	switch {
	case errors.Is(err, ErrNoExtension):
		notice = "This demo cannot be extended any more."
	case err != nil:
		log.Printf("cloud: a demo was not extended: %v", err)
		notice = "The demo could not be extended."
	}
	if notice == "" {
		http.Redirect(w, r, "/demo/wait", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/demo/wait?notice="+strings.ReplaceAll(notice, " ", "+"), http.StatusSeeOther)
}

// demoEnded is what an address shows after its demo has been deleted: that it has ended, and where to book a call.
func (s *Server) demoEnded(w http.ResponseWriter, slug string) bool {
	lead, found := s.Store.LeadOf(slug)
	if !found || lead.Ended.IsZero() {
		return false
	}
	s.render(w, http.StatusGone, "demo-end", view{Title: "Your haas demo has ended", Ended: true, Name: lead.Name, Book: s.Store.set.Demo.BookURL})
	return true
}

// demoLeaseJSON says how long a demo has, for the banner on each of its pages, and where to extend it and book a call.
func (s *Server) demoLeaseJSON(w http.ResponseWriter, slug string) bool {
	demo, left, found := s.Store.DemoLeft(slug)
	if !found || demo.Expires.IsZero() {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `{"seconds":%d,"demo":true,"extend":%q,"book":%q}`+"\n", int(left/time.Second), s.URL.JoinPath("/demo/wait").String(), s.Store.set.Demo.BookURL)
	return true
}
