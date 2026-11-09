package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PayPalSandbox is the only PayPal this code talks to. There is no setting that points it at the live service,
// so a live credential put in the environment fails to sign in here and moves no money.
const PayPalSandbox = "https://api-m.sandbox.paypal.com"

const (
	payPalTimeout   = 15 * time.Second
	payPalReplyMax  = 1 << 20
	tokenEarlyAfter = time.Minute
)

// PayPalConfig is a sandbox app's credentials and the id of the webhook registered for this server.
type PayPalConfig struct {
	ClientID, ClientSecret, WebhookID string
}

// Ready says the configuration holds everything the billing needs.
func (c PayPalConfig) Ready() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.WebhookID != ""
}

// PayPalClient calls the sandbox's REST API: the subscription calls and the webhook check. It holds the
// credentials and an access token it renews, and handles no card: the payer enters theirs at PayPal.
type PayPalClient struct {
	cfg  PayPalConfig
	base string
	http *http.Client
	now  func() time.Time

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewPayPalClient makes a client of the sandbox.
func NewPayPalClient(cfg PayPalConfig) *PayPalClient {
	return &PayPalClient{cfg: cfg, base: PayPalSandbox, http: &http.Client{Timeout: payPalTimeout}, now: time.Now}
}

// PayPalError is a refusal PayPal sent: its status, its name for the problem, and its debug id, which
// PayPal's support asks for.
type PayPalError struct {
	Status  int    `json:"-"`
	Name    string `json:"name"`
	Message string `json:"message"`
	DebugID string `json:"debug_id"`
}

func (e *PayPalError) Error() string {
	return fmt.Sprintf("paypal answered %d %s: %s (debug id %s)", e.Status, e.Name, e.Message, e.DebugID)
}

// call sends one request with a JSON body and reads the JSON answer into out when there is one. A request id
// makes PayPal treat a repeat of the call as the same call.
func (c *PayPalClient) call(ctx context.Context, method, path, requestID string, body []byte, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")
	if requestID != "" {
		req.Header.Set("PayPal-Request-Id", requestID)
	}
	return c.do(req, out)
}

func (c *PayPalClient) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reply, err := io.ReadAll(io.LimitReader(resp.Body, payPalReplyMax))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		refusal := &PayPalError{Status: resp.StatusCode}
		_ = json.Unmarshal(reply, refusal)
		return refusal.withDebugID(resp)
	}
	if out == nil || len(bytes.TrimSpace(reply)) == 0 {
		return nil
	}
	return json.Unmarshal(reply, out)
}

func (e *PayPalError) withDebugID(resp *http.Response) *PayPalError {
	if e.DebugID == "" {
		e.DebugID = resp.Header.Get("Paypal-Debug-Id")
	}
	return e
}

// accessToken is the OAuth token for the app, fetched with the client id and secret and kept until a minute
// before it ends. The lock is held across the fetch so that a burst of calls makes one.
func (c *PayPalClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.expires) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/oauth2/token", strings.NewReader(url.Values{"grant_type": {"client_credentials"}}.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var grant struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.do(req, &grant); err != nil {
		return "", err
	}
	if grant.AccessToken == "" {
		return "", errors.New("paypal gave no access token")
	}
	c.token = grant.AccessToken
	c.expires = c.now().Add(time.Duration(grant.ExpiresIn)*time.Second - tokenEarlyAfter)
	return c.token, nil
}

// PayPalSubscription is what gusto reads of a PayPal subscription. The payer's name and address are in PayPal's answer
// and are never read into it.
type PayPalSubscription struct {
	ID             string
	PlanID         string
	Status         string
	Quantity       string
	CustomID       string
	UpdateTime     time.Time
	FailedPayments int
	ApproveURL     string
}

// Seats is the quantity as a number, and fallback when PayPal sent none.
func (s PayPalSubscription) Seats(fallback int) int {
	if n, err := strconv.Atoi(s.Quantity); err == nil && n > 0 {
		return n
	}
	return fallback
}

type wireSubscription struct {
	ID          string    `json:"id"`
	PlanID      string    `json:"plan_id"`
	Status      string    `json:"status"`
	Quantity    string    `json:"quantity"`
	CustomID    string    `json:"custom_id"`
	UpdateTime  time.Time `json:"update_time"`
	BillingInfo struct {
		FailedPayments int `json:"failed_payments_count"`
	} `json:"billing_info"`
	Links []wireLink `json:"links"`
}

type wireLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

func approveLink(links []wireLink) string {
	for _, link := range links {
		if link.Rel == "approve" {
			return link.Href
		}
	}
	return ""
}

func (w wireSubscription) read() PayPalSubscription {
	return PayPalSubscription{ID: w.ID, PlanID: w.PlanID, Status: w.Status, Quantity: w.Quantity, CustomID: w.CustomID,
		UpdateTime: w.UpdateTime, FailedPayments: w.BillingInfo.FailedPayments, ApproveURL: approveLink(w.Links)}
}

// Subscription is PayPal's current record of a subscription.
func (c *PayPalClient) Subscription(ctx context.Context, id string) (PayPalSubscription, error) {
	var wire wireSubscription
	err := c.call(ctx, http.MethodGet, "/v1/billing/subscriptions/"+url.PathEscape(id), "", nil, &wire)
	return wire.read(), err
}

// CreateSubscription makes a subscription to a plan for a number of seats, tagged with the organization, and
// returns it with the address the payer approves it at. requestID makes a repeat of the call return the same one.
func (c *PayPalClient) CreateSubscription(ctx context.Context, planID string, seats int, org, returnURL, cancelURL, requestID string) (PayPalSubscription, error) {
	body, err := json.Marshal(map[string]any{
		"plan_id": planID, "quantity": strconv.Itoa(seats), "custom_id": org,
		"application_context": map[string]string{"brand_name": "gusto", "user_action": "SUBSCRIBE_NOW", "shipping_preference": "NO_SHIPPING", "return_url": returnURL, "cancel_url": cancelURL},
	})
	if err != nil {
		return PayPalSubscription{}, err
	}
	var wire wireSubscription
	if err := c.call(ctx, http.MethodPost, "/v1/billing/subscriptions", requestID, body, &wire); err != nil {
		return PayPalSubscription{}, err
	}
	return wire.read(), nil
}

// ReviseSeats asks PayPal to change a subscription's quantity. The change takes effect when the payer approves
// it, at the address returned, which is empty when PayPal needed no approval.
func (c *PayPalClient) ReviseSeats(ctx context.Context, id string, seats int, returnURL, cancelURL string) (approveURL string, err error) {
	body, err := json.Marshal(map[string]any{"quantity": strconv.Itoa(seats),
		"application_context": map[string]string{"brand_name": "gusto", "shipping_preference": "NO_SHIPPING", "return_url": returnURL, "cancel_url": cancelURL}})
	if err != nil {
		return "", err
	}
	var reply struct {
		Links []wireLink `json:"links"`
	}
	err = c.call(ctx, http.MethodPost, "/v1/billing/subscriptions/"+url.PathEscape(id)+"/revise", "", body, &reply)
	return approveLink(reply.Links), err
}

// Cancel ends a subscription.
func (c *PayPalClient) Cancel(ctx context.Context, id, reason string) error {
	body, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, "/v1/billing/subscriptions/"+url.PathEscape(id)+"/cancel", "", body, nil)
}

// WebhookHeaders are the five headers PayPal signs a delivery with.
type WebhookHeaders struct{ Algo, CertURL, ID, Signature, Time string }

func webhookHeaders(h http.Header) WebhookHeaders {
	return WebhookHeaders{Algo: h.Get("Paypal-Auth-Algo"), CertURL: h.Get("Paypal-Cert-Url"), ID: h.Get("Paypal-Transmission-Id"),
		Signature: h.Get("Paypal-Transmission-Sig"), Time: h.Get("Paypal-Transmission-Time")}
}

func (h WebhookHeaders) complete() bool {
	return h.Algo != "" && h.CertURL != "" && h.ID != "" && h.Signature != "" && h.Time != ""
}

// VerifyWebhook asks PayPal whether a delivery is one it sent to this server's webhook. The body is sent back
// byte for byte, since PayPal checks the signature against it as received.
func (c *PayPalClient) VerifyWebhook(ctx context.Context, h WebhookHeaders, body []byte) (bool, error) {
	out := []byte("{")
	for _, part := range [][2]string{{"auth_algo", h.Algo}, {"cert_url", h.CertURL}, {"transmission_id", h.ID},
		{"transmission_sig", h.Signature}, {"transmission_time", h.Time}, {"webhook_id", c.cfg.WebhookID}} {
		out = append(out, quoted(part[0])...)
		out = append(out, ':')
		out = append(out, quoted(part[1])...)
		out = append(out, ',')
	}
	out = append(out, `"webhook_event":`...)
	out = append(out, bytes.TrimSpace(body)...)
	out = append(out, '}')
	var reply struct {
		Status string `json:"verification_status"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/notifications/verify-webhook-signature", "", out, &reply); err != nil {
		return false, err
	}
	return reply.Status == "SUCCESS", nil
}

func quoted(s string) []byte {
	text, _ := json.Marshal(s)
	return text
}

// Price of a seat, as the plan states it.
const (
	SeatPrice    = "25.00"
	SeatCurrency = "USD"
)

// EnsurePlan makes the product and the plan of gusto cloud at PayPal, and returns the plan's id: $25 a seat a
// month, with a quantity, billed until cancelled. PayPal keeps a request id for 72 hours, so a repeat inside it
// returns what the first call made. The caller keeps the id, so later calls do not reach here.
func (c *PayPalClient) EnsurePlan(ctx context.Context) (string, error) {
	product, err := json.Marshal(map[string]string{"name": "gusto cloud", "description": "gusto cloud, by the seat", "type": "SERVICE", "category": "SOFTWARE"})
	if err != nil {
		return "", err
	}
	var made struct {
		ID string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/catalogs/products", "gusto-cloud-product-v1", product, &made); err != nil {
		return "", err
	}
	plan, err := json.Marshal(map[string]any{
		"product_id": made.ID, "name": "gusto cloud, $25 a seat a month", "status": "ACTIVE", "quantity_supported": true,
		"billing_cycles": []map[string]any{{"tenure_type": "REGULAR", "sequence": 1, "total_cycles": 0,
			"frequency":      map[string]any{"interval_unit": "MONTH", "interval_count": 1},
			"pricing_scheme": map[string]any{"fixed_price": map[string]string{"value": SeatPrice, "currency_code": SeatCurrency}}}},
		"payment_preferences": map[string]any{"auto_bill_outstanding": true, "payment_failure_threshold": 3, "setup_fee_failure_action": "CANCEL"},
	})
	if err != nil {
		return "", err
	}
	made.ID = ""
	if err := c.call(ctx, http.MethodPost, "/v1/billing/plans", "gusto-cloud-plan-v1", plan, &made); err != nil {
		return "", err
	}
	return made.ID, nil
}
