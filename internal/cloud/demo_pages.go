package cloud

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	page := view{Title: demoFormTitle, Book: s.Store.set.Demo.BookURL, DemoLeft: s.Store.set.Demo.Length}
	s.drawDemoForm(w, http.StatusOK, s.seen(page, "/demo", "Start a "+minuteLong(page.DemoLeft)+" demo of "+siteName+": your own organisation, with agents working in workspaces of their own, and nothing to sign up for."))
}

// drawDemoForm draws the form. Where there is a bot check its widget is on the form, and the page may load its script and
// frame its answer, from Cloudflare and from nowhere else.
func (s *Server) drawDemoForm(w http.ResponseWriter, status int, page view) {
	if s.Turnstile.Enabled() {
		page.TurnstileSite = s.Turnstile.Site
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; "+
			"script-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; style-src 'self' 'unsafe-inline'; connect-src 'self' https://challenges.cloudflare.com")
	}
	s.render(w, status, "demo", page)
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
		s.drawDemoForm(w, status, view{Title: demoFormTitle, Error: why, Email: email, OrgName: company, Name: name,
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
	if s.Turnstile.Enabled() {
		if err := s.Turnstile.Check(r.Context(), r.PostFormValue("cf-turnstile-response"), s.source(r)); err != nil {
			status := http.StatusForbidden
			if errors.Is(err, ErrCheckDown) {
				status = http.StatusServiceUnavailable
			}
			again(status, refusalSentence(err)+" Try again.")
			return
		}
	}
	person, org, err := s.Store.StartDemo(name, email, company)
	if err != nil {
		again(demoStatus(err), demoText(err))
		return
	}
	s.provision(org.Slug)
	s.tell("started", org.Slug)
	s.noticeOfDemo(org.Slug)
	s.Sessions.Issue(w, person.Email)
	http.Redirect(w, r, "/demo/wait", http.StatusSeeOther)
}

func demoText(err error) string {
	switch {
	case errors.Is(err, ErrDemoFull):
		return "Every demo is in use right now. Book a call and we will show you."
	case errors.Is(err, ErrDemoPaused), errors.Is(err, ErrDemoDaily):
		return "We are not starting new demos right now. Book a call and we will show you."
	case errors.Is(err, ErrDemoRepeat):
		return "You have had the demos this address or company gets in a day. Book a call and we will show you the rest."
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
	case errors.Is(err, ErrDemoFull), errors.Is(err, ErrDemoPaused), errors.Is(err, ErrDemoDaily):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrDemoRepeat):
		return http.StatusTooManyRequests
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
	page := view{Title: "Your gusto demo", Org: Org{Slug: person.Org, Name: demo.Company}, Person: person, DemoLeft: left,
		Book: s.Store.set.Demo.BookURL, Extendable: !demo.Extended && s.Store.set.Demo.Extend > 0 && !demo.Expires.IsZero(),
		Notice: r.URL.Query().Get("notice")}
	if state != Ready {
		page.Refresh, page.DemoWords = true, waitingWords(state)
		page.Steps = demoSteps(state)
		if lead, found := s.Store.LeadOf(person.Org); found {
			page.Waited = waited(lead.Requested, time.Now())
		}
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
	switch state {
	case Failed:
		return "Something went wrong starting your demo. It is being tried again."
	case Waiting:
		return "Starting a machine for your demo. When none is running this takes about a minute, and your time starts when it is ready, not before."
	}
	return "Getting your demo ready. When nothing else is running this takes about a minute, and your time starts when it is ready, not before."
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

// stopDemoPage asks a visitor whether to end their demo now, since everything in it is deleted with it.
func (s *Server) stopDemoPage(w http.ResponseWriter, r *http.Request, person Person) {
	if _, _, found := s.Store.DemoLeft(person.Org); !found {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "demo-stop", view{Title: "End your gusto demo"})
}

// stopDemo ends a visitor's demo now, which is what it does at the end of its time, and takes them to the page that
// says when it is done.
func (s *Server) stopDemo(w http.ResponseWriter, r *http.Request, person Person) {
	if err := s.Store.StopDemo(person.Org); err != nil {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	s.EndDueDemos()
	http.Redirect(w, r, "/demo/ended/"+person.Org, http.StatusSeeOther)
}

// demoEndedPage is where a visitor who ended their demo waits for it to be deleted, and reads that it has been.
func (s *Server) demoEndedPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	lead, found := s.Store.LeadOf(slug)
	if !found {
		http.NotFound(w, r)
		return
	}
	page := view{Title: "Your gusto demo has ended", Ended: true, Name: lead.Name, Book: s.Store.set.Demo.BookURL}
	if lead.Ended.IsZero() {
		page.Title, page.Ending, page.Refresh = "Ending your gusto demo", true, true
	}
	s.render(w, http.StatusOK, "demo-end", page)
}

// demoEnded is what an address shows after its demo has been deleted: that it has ended, and where to book a call.
func (s *Server) demoEnded(w http.ResponseWriter, slug string) bool {
	lead, found := s.Store.LeadOf(slug)
	if !found || lead.Ended.IsZero() {
		return false
	}
	s.render(w, http.StatusGone, "demo-end", view{Title: "Your gusto demo has ended", Ended: true, Name: lead.Name, Book: s.Store.set.Demo.BookURL})
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
	fmt.Fprintf(w, `{"seconds":%d,"demo":true,"extend":%q,"end":%q,"book":%q}`+"\n", int(left/time.Second), s.URL.JoinPath("/demo/wait").String(), s.URL.JoinPath("/demo/end").String(), s.Store.set.Demo.BookURL)
	return true
}

// landing is what a visitor sees first where a demo is offered: what gusto is, the video if there is one, and where
// to start a demo or book a call.
func (s *Server) landing(w http.ResponseWriter) {
	demo := s.Store.set.Demo
	page := view{Title: landingTitle, DemoLeft: demo.Length, Book: demo.BookURL}
	description := landingDescription(page)
	page = s.seen(page, "/", description)
	page.LD = s.structured(description)
	csp := "default-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'"
	if demo.VideoURL == "" {
		// A finished task and its signed record is what is shown unless an operator names a video of their own.
		page.Shot = "/static/hero.webp"
	} else if video, file, frame := videoOf(demo.VideoURL); video != "" {
		page.Video, page.VideoFile = video, file
		if file {
			csp += "; media-src 'self' https:"
		} else {
			csp += "; frame-src " + frame
		}
	}
	w.Header().Set("Content-Security-Policy", csp)
	s.render(w, http.StatusOK, "landing", page)
}

// videoOf is the video to show for an address, whether it is a file, and the origin a page for it is embedded from. An
// address that is not https is not shown.
func videoOf(address string) (video string, file bool, origin string) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", false, ""
	}
	path := strings.ToLower(parsed.Path)
	if strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".webm") {
		return parsed.String(), true, ""
	}
	return parsed.String(), false, parsed.Scheme + "://" + parsed.Host
}
