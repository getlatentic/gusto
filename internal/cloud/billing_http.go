package cloud

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

const (
	webhookPrefix       = "/billing/webhook/"
	stripeWebhookPath   = "/stripe/webhook"
	webhookBody         = 512 << 10
	billingCallTimeout  = 30 * time.Second
	deliveriesPerHour   = 2000
	agentCallsPerHour   = 600
	notAnsweringMessage = "Payments are not answering. Nothing was charged. Try again in a minute."
)

// billingRoutes adds the billing pages and the endpoints programs call. Without a processor set up none of them exist.
func (s *Server) billingRoutes(mux *http.ServeMux) {
	if !s.Billing.Enabled() {
		return
	}
	mux.HandleFunc("POST "+webhookPrefix+"{provider}", func(w http.ResponseWriter, r *http.Request) { s.deliver(w, r, r.PathValue("provider")) })
	if _, found := s.Billing.Providers.Get("stripe"); found {
		mux.HandleFunc("POST "+stripeWebhookPath, func(w http.ResponseWriter, r *http.Request) { s.deliver(w, r, "stripe") })
	}
	mux.HandleFunc("POST /billing/mcp", s.billingMCP)
	mux.HandleFunc("GET /org/billing/return", s.guard(s.billingReturn))
	mux.HandleFunc("POST /org/billing/subscribe", s.billingAction(func(ctx context.Context, p Person, r *http.Request) (string, error) {
		return s.Billing.Subscribe(ctx, p.Org, p.Email, r.PostFormValue("provider"))
	}))
	mux.HandleFunc("POST /org/billing/manage", s.billingAction(func(ctx context.Context, p Person, _ *http.Request) (string, error) {
		return s.Billing.Manage(ctx, p.Org)
	}))
	mux.HandleFunc("POST /org/billing/cancel", s.billingAction(func(ctx context.Context, p Person, _ *http.Request) (string, error) {
		return "", s.Billing.Cancel(ctx, p.Org)
	}))
	mux.HandleFunc("POST /org/billing/proposals/approve", s.billingAction(func(ctx context.Context, p Person, r *http.Request) (string, error) {
		return s.Billing.Approve(ctx, p.Org, r.PostFormValue("id"), p.Email)
	}))
	mux.HandleFunc("POST /org/billing/proposals/decline", s.billingAction(func(_ context.Context, p Person, r *http.Request) (string, error) {
		return "", s.Store.Decline(p.Org, r.PostFormValue("id"), p.Email)
	}))
	mux.HandleFunc("POST /org/billing/agent-token", s.guardBilling(s.agentToken))
}

// guardBilling is guard for what only an admin of an organization that is billed may do. A demo is never billed.
func (s *Server) guardBilling(next func(http.ResponseWriter, *http.Request, Person)) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request, person Person) {
		if org, found, err := s.Store.Org(r.Context(), person.Org); err != nil || !found || org.Demo != nil {
			http.NotFound(w, r)
			return
		}
		if person.Role != Admin {
			s.drawOrg(r.Context(), w, http.StatusForbidden, person, refusalSentence(ErrNotAdmin))
			return
		}
		next(w, r, person)
	})
}

// billingAction runs one thing an admin asks of billing and sends them to where it says, or back to the organization page.
func (s *Server) billingAction(call func(context.Context, Person, *http.Request) (string, error)) http.HandlerFunc {
	return s.guardBilling(func(w http.ResponseWriter, r *http.Request, person Person) {
		ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
		defer cancel()
		to, err := call(ctx, person, r)
		if errors.Is(err, errNoPortal) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			s.drawBillingRefusal(w, r, person, err)
			return
		}
		if to == "" {
			to = "/org?org=" + url.QueryEscape(person.Org)
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
	})
}

func (s *Server) billingReturn(w http.ResponseWriter, r *http.Request, person Person) {
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	if err := s.Billing.Return(ctx, person.Org); err != nil {
		slog.Error("a payer's return was not read", "org", person.Org, "error", err)
	}
	http.Redirect(w, r, "/org?org="+url.QueryEscape(person.Org)+"&billing=started", http.StatusSeeOther)
}

func (s *Server) agentToken(w http.ResponseWriter, r *http.Request, person Person) {
	secret, err := s.Store.IssueAgentToken(person.Org)
	if err != nil {
		s.drawBillingRefusal(w, r, person, err)
		return
	}
	s.drawOrg(r.Context(), w, http.StatusOK, person, "Agent token, shown once: "+secret)
}

// deliver receives a processor's notice. The processor checks it, and it is applied only when the processor says it is
// its own. A failure that is ours to fix is a 5xx, which makes the processor send the notice again.
func (s *Server) deliver(w http.ResponseWriter, r *http.Request, name string) {
	if _, found := s.Billing.Providers.Get(name); !found {
		http.NotFound(w, r)
		return
	}
	if !s.billingHooks.Allow(r.Context(), s.source(r)) {
		http.Error(w, "too many deliveries", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBody))
	if err != nil {
		http.Error(w, "the delivery is too large", http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), billingCallTimeout)
	defer cancel()
	switch err := s.Billing.Hook(ctx, name, payments.Delivery{Header: r.Header, Body: body}); {
	case err == nil:
		io.WriteString(w, "ok\n")
	case errors.Is(err, payments.ErrBadSignature):
		http.Error(w, payments.ErrBadSignature.Error(), http.StatusUnauthorized)
	case errors.Is(err, payments.ErrMalformed):
		http.Error(w, payments.ErrMalformed.Error(), http.StatusBadRequest)
	default:
		slog.Error("a delivery was not applied", "provider", name, "error", err)
		http.Error(w, "try again", http.StatusInternalServerError)
	}
}

// billingSentences are what a person is told of a refusal billing made.
var billingSentences = []error{ErrSubscribed, ErrNotBilled, ErrSeats, ErrProposal, ErrProposalOpen, ErrFollows, ErrUnsupported, ErrNoProvider}

func (s *Server) drawBillingRefusal(w http.ResponseWriter, r *http.Request, person Person, err error) {
	for _, known := range billingSentences {
		if errors.Is(err, known) {
			s.drawOrg(r.Context(), w, http.StatusConflict, person, refusalSentence(known))
			return
		}
	}
	slog.Error("a billing request failed", "org", person.Org, "error", err)
	s.drawOrg(r.Context(), w, http.StatusBadGateway, person, notAnsweringMessage)
}

// billingSource is where a form of the organization page may send a person: each processor's own pages.
func (s *Server) billingSource() string {
	if !s.Billing.Enabled() {
		return ""
	}
	var origins []string
	for _, p := range s.Billing.Providers.All() {
		origins = append(origins, p.Origins()...)
	}
	if len(origins) == 0 {
		return ""
	}
	return " " + strings.Join(origins, " ")
}
