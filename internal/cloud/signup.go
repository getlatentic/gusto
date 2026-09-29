package cloud

import (
	"errors"
	"log"
	"net/http"
	"strings"
)

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.person(r); ok {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, "signup", view{Title: "Create your organisation"})
}

// signUp makes the person and their organisation, signs them in, and mails them
// the link that lifts the trial. A person who was refused sees why and keeps
// what they typed.
func (s *Server) signUp(w http.ResponseWriter, r *http.Request) {
	in := Signup{Email: r.PostFormValue("email"), OrgName: r.PostFormValue("org"), Slug: r.PostFormValue("slug")}
	again := func(status int, why string, more bool) {
		s.render(w, status, "signup", view{Title: "Create your organisation", Error: why, Email: in.Email, OrgName: in.OrgName, Slug: in.Slug,
			More: more || in.OrgName != "" || in.Slug != ""})
	}
	if !s.all.Allow("all") {
		again(http.StatusServiceUnavailable, "Sign-ups are busy right now. Try again in a little while.", false)
		return
	}
	if !s.signups.Allow(s.source(r)) {
		again(http.StatusTooManyRequests, "That is a lot of sign-ups from one place. Try again later.", false)
		return
	}
	person, org, err := s.Store.SignUp(in)
	if err != nil {
		again(refusalStatus(err), refusalText(err), asksForMore(err))
		return
	}
	s.mailVerification(person, org)
	s.provision(org.Slug)
	s.Sessions.Issue(w, person.Email)
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

// asksForMore says the form needs the organisation's name and address, because
// the domain could not give them or gave ones that will not do.
func asksForMore(err error) bool {
	for _, known := range []error{ErrNeedName, ErrSameCompany, ErrBadOrgName, ErrBadSlug, ErrReservedSlug, ErrSlugTaken} {
		if errors.Is(err, known) {
			return true
		}
	}
	return false
}

// refusalSentence is a refusal as a sentence.
func refusalSentence(known error) string {
	return strings.ToUpper(known.Error()[:1]) + known.Error()[1:] + "."
}

// refusalText is what a person is told when a sign-up cannot go ahead. An
// error that is not one of ours is ours to log, and says nothing of itself.
func refusalText(err error) string {
	for _, known := range []error{ErrBadEmail, ErrFreeMail, ErrDisposable, ErrBadOrgName, ErrBadSlug, ErrReservedSlug, ErrEmailTaken, ErrSlugTaken, ErrDomainFull, ErrNeedName, ErrSameCompany} {
		if errors.Is(err, known) {
			return refusalSentence(known)
		}
	}
	log.Printf("cloud: a sign-up failed: %v", err)
	return "Something went wrong on our side. Try again."
}

func refusalStatus(err error) int {
	for _, known := range []error{ErrEmailTaken, ErrSlugTaken, ErrDomainFull} {
		if errors.Is(err, known) {
			return http.StatusConflict
		}
	}
	for _, known := range []error{ErrBadEmail, ErrFreeMail, ErrDisposable, ErrBadOrgName, ErrBadSlug, ErrReservedSlug, ErrNeedName, ErrSameCompany} {
		if errors.Is(err, known) {
			return http.StatusUnprocessableEntity
		}
	}
	return http.StatusInternalServerError
}

// mailVerification sends the link that shows an address is read. It is not
// counted against the mailbox's limit, which is for asking again.
func (s *Server) mailVerification(person Person, org Org) {
	secret, err := s.Store.MakeLink(person.Email, Verify, VerifyLife)
	if err != nil {
		log.Printf("cloud: a verification link was not made: %v", err)
		return
	}
	s.send(person.Email, "Confirm your email address for "+org.Name,
		"Follow this link to confirm your email address and lift the limits on "+org.Name+":\n\n"+
			s.link("/verify", secret)+"\n\nThe link works once and expires in three days. If you did not sign up, ignore this mail.\n")
}

func (s *Server) signInPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "signin", view{Title: "Sign in"})
}

// signIn mails a link to an address that has an account. The answer is the same
// whether or not it does, and mail is not waited for, so the form cannot be used
// to find out who is a customer.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))
	if !s.signups.Allow("signin:" + s.source(r)) {
		s.render(w, http.StatusTooManyRequests, "signin", view{Title: "Sign in", Email: email, Error: "That is a lot of attempts from one place. Try again later."})
		return
	}
	if normal, err := NormalEmail(email); err == nil && s.mails.Allow(normal) {
		if _, found, err := s.Store.Person(normal); err == nil && found {
			s.mailSignIn(normal)
		}
	}
	s.render(w, http.StatusOK, "signin", view{Title: "Sign in", Sent: true})
}

func (s *Server) mailSignIn(email string) {
	secret, err := s.Store.MakeLink(email, SignIn, SignInLife)
	if err != nil {
		log.Printf("cloud: a sign-in link was not made: %v", err)
		return
	}
	s.send(email, "Your haas sign-in link",
		"Follow this link to sign in:\n\n"+s.link("/signin/link", secret)+"\n\nThe link works once and expires in 20 minutes. If you did not ask for it, ignore this mail.\n")
}

// confirmPage asks a person to press a button before a link is used. A mail
// program that opens every link to check it would otherwise spend them all.
func (s *Server) confirmPage(purpose Purpose, title, button string) http.HandlerFunc {
	action := "/verify"
	if purpose == SignIn {
		action = "/signin/link"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusOK, "confirm", view{Title: title, Token: r.URL.Query().Get("t"), Action: action, Button: button})
	}
}

// useLink spends a link, and signs the person it was made for in.
func (s *Server) useLink(purpose Purpose) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email, err := s.Store.UseLink(r.PostFormValue("t"), purpose)
		if err != nil {
			s.render(w, http.StatusGone, "signin", view{Title: "Sign in", Error: err.Error() + "."})
			return
		}
		if purpose == Verify {
			if _, err := s.Store.Verify(email); err != nil {
				log.Printf("cloud: an address was not verified: %v", err)
				s.render(w, http.StatusInternalServerError, "signin", view{Title: "Sign in", Error: "Something went wrong on our side. Try again."})
				return
			}
		}
		s.Sessions.Issue(w, email)
		http.Redirect(w, r, "/org", http.StatusSeeOther)
	}
}
