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
	billingWebhookPath = "/billing/paypal/webhook"
	billingWebhookMax  = 256 << 10
	billingCallTimeout = 30 * time.Second
	webhooksPerHour    = 2000
)

// billingRoutes adds the billing pages and the two endpoints programs call. Without PayPal set up none of them exist.
func (s *Server) billingRoutes(mux *http.ServeMux) {
	if s.Billing == nil {
		return
	}
	mux.HandleFunc("POST "+billingWebhookPath, s.payPalWebhook)
	mux.HandleFunc("POST /billing/mcp", s.billingMCP)
	mux.HandleFunc("GET /org/billing/return", s.guard(s.billingReturn))
	mux.HandleFunc("POST /org/billing/subscribe", s.guardAdmin(s.billingSubscribe))
	mux.HandleFunc("POST /org/billing/cancel", s.guardAdmin(s.billingCancel))
	mux.HandleFunc("POST /org/billing/proposals/approve", s.guardAdmin(s.billingApprove))
	mux.HandleFunc("POST /org/billing/proposals/decline", s.guardAdmin(s.billingDecline))
	mux.HandleFunc("POST /org/billing/agent-token", s.guardAdmin(s.billingAgentToken))
}

// guardAdmin is guard for what only an admin may do.
func (s *Server) guardAdmin(next func(http.ResponseWriter, *http.Request, Person)) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request, person Person) {
		if person.Role != Admin {
			s.drawOrg(w, http.StatusForbidden, person, refusalSentence(ErrNotAdmin))
			return
		}
		next(w, r, person)
	})
}

// payPalWebhook receives PayPal's deliveries. PayPal signs each one, and it is applied only when PayPal says the
// signature is its own. A refusal that is ours to fix is a 5xx, which makes PayPal send the delivery again.
func (s *Server) payPalWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.webhooks.Allow(s.source(r)) {
		http.Error(w, "too many deliveries", http.StatusTooManyRequests)
		return
	}
	headers := webhookHeaders(r.Header)
	if !headers.complete() {
		http.Error(w, "not a PayPal delivery", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, billingWebhookMax))
	if err != nil {
		http.Error(w, "the delivery is too large", http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	switch err := s.Billing.HandleWebhook(ctx, headers, body); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrBadSignature):
		http.Error(w, "the signature is not PayPal's", http.StatusUnauthorized)
	default:
		log.Printf("cloud: a PayPal delivery was not applied: %v", err)
		http.Error(w, "try again", http.StatusInternalServerError)
	}
}

func (s *Server) billingReturn(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	if err := s.Billing.Return(ctx, person.Org); err != nil {
		log.Printf("cloud: a payer's return was not read: %v", err)
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) billingSubscribe(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	approve, err := s.Billing.Subscribe(ctx, person.Org)
	if err != nil {
		s.drawBillingRefusal(w, person, err)
		return
	}
	http.Redirect(w, r, approve, http.StatusSeeOther)
}

func (s *Server) billingCancel(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	if err := s.Billing.Cancel(ctx, person.Org); err != nil {
		s.drawBillingRefusal(w, person, err)
		return
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) billingApprove(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	approve, err := s.Billing.Approve(ctx, person.Org, r.PostFormValue("id"), person.Email)
	if err != nil {
		s.drawBillingRefusal(w, person, err)
		return
	}
	if approve == "" {
		approve = "/org"
	}
	http.Redirect(w, r, approve, http.StatusSeeOther)
}

func (s *Server) billingDecline(w http.ResponseWriter, r *http.Request, person Person) {
	if err := s.Store.Decline(person.Org, r.PostFormValue("id"), person.Email); err != nil {
		s.drawBillingRefusal(w, person, err)
		return
	}
	http.Redirect(w, r, "/org", http.StatusSeeOther)
}

func (s *Server) billingAgentToken(w http.ResponseWriter, r *http.Request, person Person) {
	secret, err := s.Store.IssueAgentToken(person.Org)
	if err != nil {
		s.drawBillingRefusal(w, person, err)
		return
	}
	s.drawOrg(w, http.StatusOK, person, "Agent token, shown once: "+secret)
}

// billingSentences are what a person is told of a refusal billing made.
var billingSentences = []error{ErrSubscribed, ErrNotBilled, ErrSeats, ErrProposal, ErrProposalOpen}

func (s *Server) drawBillingRefusal(w http.ResponseWriter, person Person, err error) {
	for _, known := range billingSentences {
		if errors.Is(err, known) {
			s.drawOrg(w, http.StatusConflict, person, refusalSentence(known))
			return
		}
	}
	log.Printf("cloud: a billing request failed: %v", err)
	s.drawOrg(w, http.StatusBadGateway, person, "PayPal could not be reached or refused the request. Nothing was charged. Try again.")
}

// billingView is what the organization page shows of billing.
type billingView struct {
	Admin     bool
	State     BillingState
	Words     string
	Seats     int
	People    int
	Price     string
	Proposals []Proposal
	Subscribe bool
	Cancel    bool
}

// billingView is nil when this server does not sell by PayPal.
func (s *Server) billingView(person Person, people int) *billingView {
	if s.Billing == nil {
		return nil
	}
	view := &billingView{Admin: person.Role == Admin, People: people, Price: "$" + SeatPrice + " a seat a month", Words: "Not subscribed", Subscribe: true}
	record, found, err := s.Store.BillingOf(person.Org)
	if err != nil {
		log.Printf("cloud: an organization's billing was not read: %v", err)
		return view
	}
	if found {
		view.State, view.Seats, view.Words = record.State, record.Seats, billingWords(record)
		view.Subscribe = record.State == BillingSuspended || record.State == BillingPending
		view.Cancel = record.State == BillingActive || record.State == BillingPastDue
	}
	if view.Admin {
		view.Proposals, _ = s.Store.OpenProposals(person.Org)
	}
	return view
}

func billingWords(r BillingRecord) string {
	seats := strconv.Itoa(r.Seats) + " seats"
	switch r.State {
	case BillingPending:
		return "Waiting for the payer to approve at PayPal"
	case BillingActive:
		return "Paid, " + seats
	case BillingPastDue:
		return "The last payment failed and PayPal is trying again. " + seats + " keep working meanwhile"
	}
	return "Suspended: no new tasks or invitations until an admin subscribes again. Nothing has been deleted"
}
