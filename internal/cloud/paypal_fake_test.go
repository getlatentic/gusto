package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePayPal answers the calls billing makes with the shapes PayPal's published OpenAPI schema gives
// (https://developer.paypal.com/api/subscriptions/v1/schema.json). It is not a recording of the sandbox:
// TestPayPalSandbox is what reaches the sandbox, when its credentials are in the environment.
type fakePayPal struct {
	t      *testing.T
	server *httptest.Server
	clock  time.Time

	mu          sync.Mutex
	subs        map[string]*fakeSub
	signed      map[string][]byte // delivery id -> the body PayPal "sent"
	calls       map[string]int
	reviseFails bool
	tokens      int
}

type fakeSub struct {
	id, status, custom, plan string
	quantity, pending        int
	failed                   int
	updated                  time.Time
}

const (
	fakeClientID = "sandbox-client"
	fakeSecret   = "sandbox-secret"
	fakeWebhook  = "WH-FAKE"
)

func newFakePayPal(t *testing.T) *fakePayPal {
	f := &fakePayPal{t: t, subs: map[string]*fakeSub{}, signed: map[string][]byte{}, calls: map[string]int{}, clock: time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakePayPal) client() *PayPalClient {
	c := NewPayPalClient(PayPalConfig{ClientID: fakeClientID, ClientSecret: fakeSecret, WebhookID: fakeWebhook})
	c.base = f.server.URL
	return c
}

func (f *fakePayPal) count(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[call]
}

func (f *fakePayPal) tick() time.Time { f.clock = f.clock.Add(time.Minute); return f.clock }

func (f *fakePayPal) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	f.calls[r.Method+" "+r.URL.Path]++
	if r.URL.Path == "/v1/oauth2/token" {
		user, pass, _ := r.BasicAuth()
		if user != fakeClientID || pass != fakeSecret {
			f.refuse(w, 401, "AUTHENTICATION_FAILURE", "Authentication failed")
			return
		}
		f.tokens++
		reply(w, 200, map[string]any{"access_token": "token-" + strconv.Itoa(f.tokens), "expires_in": 32400})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer token-") {
		f.refuse(w, 401, "AUTHENTICATION_FAILURE", "Authentication failed")
		return
	}
	f.route(w, r, body)
}

func (f *fakePayPal) refuse(w http.ResponseWriter, status int, name, message string) {
	w.Header().Set("Paypal-Debug-Id", "debug-1")
	reply(w, status, map[string]any{"name": name, "message": message, "debug_id": "debug-1"})
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (f *fakePayPal) route(w http.ResponseWriter, r *http.Request, body []byte) {
	path := r.URL.Path
	switch {
	case path == "/v1/catalogs/products":
		reply(w, 201, map[string]any{"id": "PROD-1"})
	case path == "/v1/billing/plans":
		reply(w, 201, map[string]any{"id": "P-PLAN", "status": "ACTIVE"})
	case path == "/v1/billing/subscriptions":
		f.create(w, body)
	case path == "/v1/notifications/verify-webhook-signature":
		f.verify(w, body)
	case strings.HasPrefix(path, "/v1/billing/subscriptions/"):
		f.subscription(w, r, strings.TrimPrefix(path, "/v1/billing/subscriptions/"), body)
	default:
		f.refuse(w, 404, "RESOURCE_NOT_FOUND", "no such path")
	}
}

func (f *fakePayPal) create(w http.ResponseWriter, body []byte) {
	var in struct {
		PlanID, Quantity, CustomID string
		Context                    map[string]string `json:"application_context"`
	}
	in.PlanID = ""
	var raw map[string]any
	json.Unmarshal(body, &raw)
	quantity, _ := strconv.Atoi(fmt.Sprint(raw["quantity"]))
	id := "I-SUB" + strconv.Itoa(len(f.subs)+1)
	f.subs[id] = &fakeSub{id: id, status: "APPROVAL_PENDING", custom: fmt.Sprint(raw["custom_id"]), plan: fmt.Sprint(raw["plan_id"]), quantity: quantity, updated: f.tick()}
	reply(w, 201, f.wire(f.subs[id], []map[string]string{{"rel": "approve", "href": "https://www.sandbox.paypal.com/webapps/billing/subscriptions?ba_token=" + id}}))
}

func (f *fakePayPal) subscription(w http.ResponseWriter, r *http.Request, rest string, body []byte) {
	id, action, _ := strings.Cut(rest, "/")
	sub, found := f.subs[id]
	if !found {
		f.refuse(w, 404, "RESOURCE_NOT_FOUND", "no such subscription")
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		reply(w, 200, f.wire(sub, nil))
	case action == "revise":
		f.revise(w, sub, body)
	case action == "cancel":
		sub.status, sub.updated = "CANCELLED", f.tick()
		w.WriteHeader(204)
	default:
		f.refuse(w, 404, "RESOURCE_NOT_FOUND", "no such action")
	}
}

func (f *fakePayPal) revise(w http.ResponseWriter, sub *fakeSub, body []byte) {
	if f.reviseFails {
		f.refuse(w, 422, "UNPROCESSABLE_ENTITY", "the subscription cannot be revised")
		return
	}
	var in struct{ Quantity string }
	json.Unmarshal(body, &in)
	sub.pending, _ = strconv.Atoi(in.Quantity)
	reply(w, 200, map[string]any{"plan_id": sub.plan, "quantity": in.Quantity, "plan_overridden": false,
		"links": []map[string]string{{"rel": "approve", "href": "https://www.sandbox.paypal.com/webapps/billing/subscriptions/update?ba_token=" + sub.id}}})
}

// wire is a subscription as PayPal writes it, with the payer's details that gusto must not read.
func (f *fakePayPal) wire(s *fakeSub, links []map[string]string) map[string]any {
	return map[string]any{"id": s.id, "plan_id": s.plan, "status": s.status, "quantity": strconv.Itoa(s.quantity), "custom_id": s.custom,
		"update_time": s.updated.Format(time.RFC3339), "links": links,
		"subscriber":   map[string]any{"email_address": "payer@example.com", "name": map[string]string{"given_name": "Pat"}},
		"billing_info": map[string]any{"failed_payments_count": s.failed, "outstanding_balance": map[string]string{"value": "0.00", "currency_code": "USD"}}}
}

// verify says SUCCESS only for a delivery this fake signed, whose body came back exactly as it went out.
func (f *fakePayPal) verify(w http.ResponseWriter, body []byte) {
	var in struct {
		ID      string          `json:"transmission_id"`
		Sig     string          `json:"transmission_sig"`
		Webhook string          `json:"webhook_id"`
		Event   json.RawMessage `json:"webhook_event"`
	}
	json.Unmarshal(body, &in)
	status := "FAILURE"
	if sent, ok := f.signed[in.ID]; ok && in.Sig == "sig-"+in.ID && in.Webhook == fakeWebhook && bytes.Equal(sent, in.Event) {
		status = "SUCCESS"
	}
	reply(w, 200, map[string]string{"verification_status": status})
}

// deliver posts an event to the front door's webhook the way PayPal does and returns the status and the id.
func (f *fakePayPal) deliver(endpoint, id, kind string, resource map[string]any) int {
	f.t.Helper()
	body := fmt.Sprintf(`{"id":%q,"event_version":"1.0",  "event_type":%q,"resource":%s}`, id, kind, mustJSON(resource))
	f.mu.Lock()
	f.signed[id] = []byte(body)
	f.mu.Unlock()
	return post(f.t, endpoint, "sig-"+id, id, []byte(body))
}

func post(t *testing.T, endpoint, sig, id string, body []byte) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Paypal-Auth-Algo", "SHA256withRSA")
	req.Header.Set("Paypal-Cert-Url", "https://api.sandbox.paypal.com/v1/notifications/certs/CERT-1")
	req.Header.Set("Paypal-Transmission-Id", id)
	req.Header.Set("Paypal-Transmission-Sig", sig)
	req.Header.Set("Paypal-Transmission-Time", "2026-11-08T09:00:00Z")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func mustJSON(v any) string {
	out, _ := json.Marshal(v)
	return string(out)
}

// These move a subscription the way PayPal does, and say what to deliver.
func (f *fakePayPal) set(id string, change func(*fakeSub)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f.subs[id])
	f.subs[id].updated = f.tick()
}

func (f *fakePayPal) activate(id string) { f.set(id, func(s *fakeSub) { s.status = "ACTIVE" }) }
func (f *fakePayPal) failPayment(id string) {
	f.set(id, func(s *fakeSub) { s.failed = 1 })
}
func (f *fakePayPal) recover(id string) { f.set(id, func(s *fakeSub) { s.failed = 0 }) }
func (f *fakePayPal) suspend(id string) { f.set(id, func(s *fakeSub) { s.status = "SUSPENDED" }) }
func (f *fakePayPal) payerApproves(id string) {
	f.set(id, func(s *fakeSub) { s.quantity, s.pending = s.pending, 0 })
}
