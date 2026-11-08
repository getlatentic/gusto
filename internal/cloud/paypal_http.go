package cloud

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	payPalWebhookPath       = "/paypal/webhook"
	payPalWebhookMax        = 256 << 10
	payPalCallTimeout       = 30 * time.Second
	payPalDeliveriesPerHour = 2000
)

// payPalRoutes adds the billing pages and the two endpoints programs call. Without PayPal set up none of them exist.
func (s *Server) payPalRoutes(mux *http.ServeMux) {
	if s.PayPal == nil {
		return
	}
	mux.HandleFunc("POST "+payPalWebhookPath, s.payPalWebhook)
	mux.HandleFunc("POST /billing/mcp", s.payPalMCP)
	mux.HandleFunc("GET /org/paypal/return", s.guard(s.payPalReturn))
	mux.HandleFunc("POST /org/paypal/subscribe", s.guardAdmin(s.payPalSubscribe))
	mux.HandleFunc("POST /org/paypal/cancel", s.guardAdmin(s.payPalCancel))
	mux.HandleFunc("POST /org/paypal/proposals/approve", s.guardAdmin(s.payPalApprove))
	mux.HandleFunc("POST /org/paypal/proposals/decline", s.guardAdmin(s.payPalDecline))
	mux.HandleFunc("POST /org/paypal/agent-token", s.guardAdmin(s.payPalAgentToken))
}

// guardAdmin is guard for what only an admin may do.
func (s *Server) guardAdmin(next func(http.ResponseWriter, *http.Request, Person)) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request, person Person) {
		if person.Role != Admin {
			s.drawOrg(r.Context(), w, http.StatusForbidden, person, refusalSentence(ErrNotAdmin))
			return
		}
		next(w, r, person)
	})
}

// payPalWebhook receives PayPal's deliveries. PayPal signs each one, and it is applied only when PayPal says the
// signature is its own. A refusal that is ours to fix is a 5xx, which makes PayPal send the delivery again.
func (s *Server) payPalWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.payPalHooks.Allow(r.Context(), s.source(r)) {
		http.Error(w, "too many deliveries", http.StatusTooManyRequests)
		return
	}
	headers := webhookHeaders(r.Header)
	if !headers.complete() {
		http.Error(w, "not a PayPal delivery", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, payPalWebhookMax))
	if err != nil {
		http.Error(w, "the delivery is too large", http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), payPalCallTimeout)
	defer cancel()
	switch err := s.PayPal.HandleWebhook(ctx, headers, body); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrBadSignature):
		http.Error(w, "the signature is not PayPal's", http.StatusUnauthorized)
	default:
		log.Printf("cloud: a PayPal delivery was not applied: %v", err)
		http.Error(w, "try again", http.StatusInternalServerError)
	}
}

func (s *Server) payPalReturn(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), payPalCallTimeout)
	defer cancel()
	if err := s.PayPal.Return(ctx, person.Org); err != nil {
		log.Printf("cloud: a payer's return was not read: %v", err)
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) payPalSubscribe(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), payPalCallTimeout)
	defer cancel()
	approve, err := s.PayPal.Subscribe(ctx, person.Org)
	if err != nil {
		s.drawPayPalRefusal(w, r, person, err)
		return
	}
	http.Redirect(w, r, approve, http.StatusSeeOther)
}

func (s *Server) payPalCancel(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), payPalCallTimeout)
	defer cancel()
	if err := s.PayPal.Cancel(ctx, person.Org); err != nil {
		s.drawPayPalRefusal(w, r, person, err)
		return
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) payPalApprove(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), payPalCallTimeout)
	defer cancel()
	approve, err := s.PayPal.Approve(ctx, person.Org, r.PostFormValue("id"), person.Email)
	if err != nil {
		s.drawPayPalRefusal(w, r, person, err)
		return
	}
	if approve == "" {
		approve = "/org"
	}
	http.Redirect(w, r, approve, http.StatusSeeOther)
}

func (s *Server) payPalDecline(w http.ResponseWriter, r *http.Request, person Person) {
	if err := s.Store.Decline(person.Org, r.PostFormValue("id"), person.Email); err != nil {
		s.drawPayPalRefusal(w, r, person, err)
		return
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) payPalAgentToken(w http.ResponseWriter, r *http.Request, person Person) {
	secret, err := s.Store.IssueAgentToken(person.Org)
	if err != nil {
		s.drawPayPalRefusal(w, r, person, err)
		return
	}
	s.drawOrg(r.Context(), w, http.StatusOK, person, "Agent token, shown once: "+secret)
}

// payPalSentences are what a person is told of a refusal billing made.
var payPalSentences = []error{ErrSubscribed, ErrNotBilled, ErrSeats, ErrProposal, ErrProposalOpen}

func (s *Server) drawPayPalRefusal(w http.ResponseWriter, r *http.Request, person Person, err error) {
	for _, known := range payPalSentences {
		if errors.Is(err, known) {
			s.drawOrg(r.Context(), w, http.StatusConflict, person, refusalSentence(known))
			return
		}
	}
	log.Printf("cloud: a billing request failed: %v", err)
	s.drawOrg(r.Context(), w, http.StatusBadGateway, person, "PayPal could not be reached or refused the request. Nothing was charged. Try again.")
}

// payPalView is what the organization page shows of billing.
type payPalView struct {
	Admin     bool
	State     PayPalState
	Words     string
	Seats     int
	People    int
	Price     string
	Proposals []Proposal
	Subscribe bool
	Cancel    bool
}

// payPalView is nil when this server does not sell by PayPal.
func (s *Server) payPalView(person Person, people int) *payPalView {
	if s.PayPal == nil {
		return nil
	}
	view := &payPalView{Admin: person.Role == Admin, People: people, Price: "$" + SeatPrice + " a seat a month", Words: "Not subscribed", Subscribe: true}
	record, found, err := s.Store.PayPalOf(person.Org)
	if err != nil {
		log.Printf("cloud: an organization's billing was not read: %v", err)
		return view
	}
	if found {
		view.State, view.Seats, view.Words = record.State, record.Seats, payPalWords(record)
		view.Subscribe = record.State == PayPalSuspended || record.State == PayPalPending
		view.Cancel = record.State == PayPalActive || record.State == PayPalPastDue
	}
	if view.Admin {
		view.Proposals, _ = s.Store.OpenProposals(person.Org)
	}
	return view
}

func payPalWords(r PayPalRecord) string {
	seats := strconv.Itoa(r.Seats) + " seats"
	switch r.State {
	case PayPalPending:
		return "Waiting for the payer to approve at PayPal"
	case PayPalActive:
		return "Paid, " + seats
	case PayPalPastDue:
		return "The last payment failed and PayPal is trying again. " + seats + " keep working meanwhile"
	}
	return "Suspended: no new tasks or invitations until an admin subscribes again. Nothing has been deleted"
}

// payPalSource is where a form of the organization page may send a person: PayPal's sandbox, when PayPal is set up.
func (s *Server) payPalSource() string {
	if s.PayPal == nil {
		return ""
	}
	return " https://www.sandbox.paypal.com"
}
