package cloud

import (
	"errors"
	"log"
	"net/http"

	"github.com/getlatentic/gusto/internal/changelog"
)

// invite sends an invitation to join the organization, from an admin. It is mailed
// once, and asking again for the same address replaces the earlier link.
func (s *Server) invite(w http.ResponseWriter, r *http.Request, person Person) {
	if !s.invites.Allow(person.Org) {
		s.drawOrg(w, http.StatusTooManyRequests, person, "That is a lot of invitations for one hour. Try again later.")
		return
	}
	address := r.PostFormValue("email")
	secret, err := s.Store.Invite(person.Org, person, address)
	if err != nil {
		s.drawOrg(w, refusedStatus(err), person, sentence(err))
		return
	}
	email, _ := NormalEmail(address)
	s.record(r, person, changelog.MemberInvited, map[string]string{"email": email, "role": string(Runner)})
	if !s.mails.Allow(email) {
		s.drawOrg(w, http.StatusTooManyRequests, person, "That address has been mailed too often. Try again later.")
		return
	}
	org, _, _ := s.Store.Org(person.Org)
	s.send(email, person.Email+" invited you to "+org.Name+" on gusto",
		person.Email+" invited you to join "+org.Name+" on gusto:\n\n"+s.link("/invite", secret)+
			"\n\nThe link works once and expires in seven days. If you were not expecting it, ignore this mail.\n")
	http.Redirect(w, r, "/org?sent=1", http.StatusSeeOther)
}

func (s *Server) cancelInvitation(w http.ResponseWriter, r *http.Request, person Person) {
	if err := s.Store.Cancel(person.Org, person, r.PostFormValue("email")); err != nil {
		s.drawOrg(w, refusedStatus(err), person, sentence(err))
		return
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

// removePerson takes a person out of the organization. Their sign-in at the front door
// stops at once, since what it says is looked up on each request, and the organization's
// own server stops trusting them within the hour it last did.
func (s *Server) removePerson(w http.ResponseWriter, r *http.Request, person Person) {
	email, _ := NormalEmail(r.PostFormValue("email"))
	target, _, _ := s.Store.Person(email, person.Org)
	if err := s.Store.Remove(person.Org, person, email); err != nil {
		s.drawOrg(w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.MemberRemoved, map[string]string{"email": email, "role": string(target.Role)})
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

// changeRole makes a person of the organization an admin or a runner. A role that is
// the one they have changes nothing and is not recorded.
func (s *Server) changeRole(w http.ResponseWriter, r *http.Request, person Person) {
	email, _ := NormalEmail(r.PostFormValue("email"))
	role := Role(r.PostFormValue("role"))
	was, err := s.Store.SetRole(person.Org, person, email, role)
	if err != nil {
		s.drawOrg(w, refusedStatus(err), person, sentence(err))
		return
	}
	if was != role {
		s.record(r, person, changelog.MemberRole, map[string]string{"email": email, "was": string(was), "now": string(role)})
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

// invitationPage asks a person to press a button to join, and says what they are
// joining. Following the link does not answer it, so a mail program that opens every
// link does not.
func (s *Server) invitationPage(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("t")
	made, found := s.Store.peekInvitation(secret)
	if !found {
		s.render(w, http.StatusGone, "signin", view{Title: "Sign in", Error: ErrLink.Error() + "."})
		return
	}
	org, _, _ := s.Store.Org(made.Org)
	s.render(w, http.StatusOK, "confirm", view{Title: "Join " + org.Name, Token: secret, Action: "/invite", Button: "Join", Inviter: made.By})
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	person, _, err := s.Store.Accept(r.PostFormValue("t"))
	switch {
	case err == nil:
		s.Sessions.Issue(w, person.Email)
		s.Sessions.Choose(w, person.Org)
		http.Redirect(w, r, s.afterJoining(person), http.StatusSeeOther)
	case errors.Is(err, ErrLink), errors.Is(err, ErrFull), errors.Is(err, ErrEmailTaken):
		text := sentence(err)
		if errors.Is(err, ErrEmailTaken) {
			text = "That address is in this organization already, so sign in instead."
		}
		s.render(w, http.StatusGone, "signin", view{Title: "Sign in", Error: text})
	default:
		log.Printf("cloud: an invitation was not accepted: %v", err)
		s.render(w, http.StatusInternalServerError, "signin", view{Title: "Sign in", Error: "Something went wrong on our side. Try again."})
	}
}

// sentence is what a person is told of a refusal we made, and nothing of one we did not.
func sentence(err error) string {
	for _, known := range []error{ErrBadEmail, ErrDisposable, ErrNoInvites, ErrFull, ErrCannotInvite, ErrNotAdmin, ErrLastAdmin, ErrNoMember, ErrBadRole, ErrLink} {
		if errors.Is(err, known) {
			return refusalSentence(known)
		}
	}
	log.Printf("cloud: an admin's request failed: %v", err)
	return "Something went wrong on our side. Try again."
}

func refusedStatus(err error) int {
	switch {
	case errors.Is(err, ErrNotAdmin):
		return http.StatusForbidden
	case errors.Is(err, ErrFull), errors.Is(err, ErrNoInvites), errors.Is(err, ErrCannotInvite), errors.Is(err, ErrLastAdmin):
		return http.StatusConflict
	case errors.Is(err, ErrBadEmail), errors.Is(err, ErrDisposable), errors.Is(err, ErrNoMember), errors.Is(err, ErrBadRole):
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}
