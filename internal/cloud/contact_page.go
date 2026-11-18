package cloud

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// contactForm is what the landing page's note to the team shows: what was typed, why it was refused, or that it was sent.
type contactForm struct {
	Error, Email, Company, Note string
	Sent                        bool
}

// contactSent is where a visitor is sent once their note is kept, so that a reload does not send it again.
const contactSent = "/?contact=sent#contact"

// sendContact keeps a visitor's note to the team and mails it to the address that follows demos up. A refused note is
// answered with the landing page and what was typed.
func (s *Server) sendContact(w http.ResponseWriter, r *http.Request) {
	if !s.demoOffered() {
		http.NotFound(w, r)
		return
	}
	typed := contactForm{Email: r.PostFormValue("email"), Company: r.PostFormValue("company"), Note: r.PostFormValue("note")}
	again := func(status int, why string) {
		typed.Error = why
		_, signedIn := s.memberships(r)
		s.landing(w, status, signedIn, typed)
	}
	if !s.signups.Allow(r.Context(), "contact:"+s.source(r)) {
		again(http.StatusTooManyRequests, "That is a lot of notes from one place. Try again later, or book a call.")
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
	contact, err := s.Store.SaveContact(r.Context(), typed.Email, typed.Company, typed.Note)
	if err != nil {
		again(contactStatus(err), contactText(err))
		return
	}
	s.noticeOfContact(contact)
	http.Redirect(w, r, contactSent, http.StatusSeeOther)
}

func contactText(err error) string {
	if errors.Is(err, ErrFreeMail) {
		return "Use the email address your company gave you."
	}
	for _, known := range []error{ErrNeedNote, ErrNoteTooLong, ErrContactRepeat, ErrContactsFull, ErrNeedCompany, ErrBadEmail, ErrDisposable, ErrBadOrgName} {
		if errors.Is(err, known) {
			return refusalSentence(known)
		}
	}
	slog.Error("a note to the team could not be kept", "error", err)
	return "Something went wrong on our side. Try again, or book a call."
}

func contactStatus(err error) int {
	switch {
	case errors.Is(err, ErrContactRepeat), errors.Is(err, ErrContactsFull):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrNeedNote), errors.Is(err, ErrNoteTooLong), errors.Is(err, ErrNeedCompany), errors.Is(err, ErrBadEmail),
		errors.Is(err, ErrFreeMail), errors.Is(err, ErrDisposable), errors.Is(err, ErrBadOrgName):
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

// noticeOfContact mails a note to the address that follows demos up, when there is one. The note is kept whether or not the
// mail goes, and the operator's page lists it. The visitor's words are in the subject on one line, so they add no header.
func (s *Server) noticeOfContact(c Contact) {
	if s.Notify == "" {
		return
	}
	body := strings.Join([]string{
		"Someone wrote to the gusto team from the landing page. Reply to them at the address below.",
		"",
		"Email: " + oneLine(c.Email),
		"Company: " + oneLine(c.Company),
		"Sent at: " + c.Sent.UTC().Format(time.RFC1123),
		"",
		c.Note,
	}, "\n")
	s.send(s.Notify, "Note from "+oneLine(c.Company)+": "+oneLine(c.Email), body)
}
