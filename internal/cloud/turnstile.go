package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Turnstile is the bot check on the demo form: Cloudflare's, which asks the visitor's browser to prove itself without
// asking the visitor anything. The form carries Site, a public key; the answer the browser puts in the form is checked
// with Cloudflare using Secret, which is never sent to a browser.
type Turnstile struct {
	Site, Secret string
	// Verify is where an answer is checked, which is Cloudflare's when empty, and Client how.
	Verify string
	Client *http.Client
}

const turnstileVerify = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Enabled says the form has the check. A check that is half set up is an error where the front door starts, and not a
// form that quietly asks nothing.
func (t Turnstile) Enabled() bool { return t.Site != "" && t.Secret != "" }

// Complete says whether one of the two is set without the other.
func (t Turnstile) Complete() error {
	if (t.Site == "") != (t.Secret == "") {
		return errors.New("the bot check needs both its site key and its secret")
	}
	return nil
}

// ErrNotHuman is an answer that Cloudflare did not accept, or that was not given.
var ErrNotHuman = errors.New("we could not tell that you are not a robot")

// ErrCheckDown is a check that could not be made. The form fails closed: a visitor who cannot be checked is asked to try
// again, and is not let through.
var ErrCheckDown = errors.New("the check that you are not a robot could not be made")

// Check says whether the answer the visitor's browser gave is one Cloudflare accepts, for the address they came from.
func (t Turnstile) Check(ctx context.Context, answer, from string) error {
	answer = strings.TrimSpace(answer)
	if answer == "" || len(answer) > 2048 {
		return ErrNotHuman
	}
	address := t.Verify
	if address == "" {
		address = turnstileVerify
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	form := url.Values{"secret": {t.Secret}, "response": {answer}}
	if from != "" {
		form.Set("remoteip", from)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrCheckDown
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return ErrCheckDown
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrCheckDown
	}
	var verdict struct {
		Success bool `json:"success"`
	}
	if json.NewDecoder(http.MaxBytesReader(nil, response.Body, 16<<10)).Decode(&verdict) != nil {
		return ErrCheckDown
	}
	if !verdict.Success {
		return ErrNotHuman
	}
	return nil
}
