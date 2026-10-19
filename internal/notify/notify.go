// Package notify tells people and systems how a routine's run ended: a signed request to an address the operator names, and
// mail to the person who added the routine.
package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Event is how one run of a routine ended.
type Event struct {
	Name     string `json:"event"`
	Instance string `json:"instance"`
	Routine  string `json:"routine"`
	Title    string `json:"title"`
	Job      string `json:"job"`
	// State is finished, failed or interrupted.
	State string `json:"state"`
	At    int64  `json:"at"`
	// Link is where the run is read, when the server knows its own address.
	Link string `json:"link,omitempty"`
	// Who added the routine, and is mailed.
	Who string `json:"who,omitempty"`
	// Answer is the end of what the agent said. It goes in the mail and never in the request to an address.
	Answer string `json:"-"`
}

// Notifier takes an event, and says whether it was taken. An event that was not is offered again. Channel is its name, kept
// with the event once it has taken it, so that a retry for another channel does not send it twice.
type Notifier interface {
	Channel() string
	Notify(Event) error
}

// Webhook sends an event as JSON, signed so the receiver knows who sent it: the header X-Gusto-Signature holds sha256= and
// the HMAC-SHA256 of the body under Secret.
type Webhook struct {
	URL, Secret string
	Client      *http.Client
}

// Check says the address is one an event may be sent to and the secret is set.
func (w Webhook) Check() error {
	parsed, err := url.Parse(w.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("the notification address is an https address")
	}
	if w.Secret == "" {
		return errors.New("the notification address needs its secret")
	}
	return nil
}

func (Webhook) Channel() string { return "webhook" }

func (w Webhook) Notify(event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	signed := hmac.New(sha256.New, []byte(w.Secret))
	signed.Write(body)
	request, err := http.NewRequest(http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Gusto-Signature", "sha256="+hex.EncodeToString(signed.Sum(nil)))
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	answer, err := client.Do(request)
	if err != nil {
		return err
	}
	defer answer.Body.Close()
	io.Copy(io.Discard, io.LimitReader(answer.Body, 1<<16))
	if answer.StatusCode/100 != 2 {
		return fmt.Errorf("%s answered %d", w.URL, answer.StatusCode)
	}
	return nil
}

// Sender sends one plain message to one address.
type Sender interface {
	Send(to, subject, body string) error
}

// Mail sends the person who added a routine how a run of it went, when the person is an address.
type Mail struct{ Via Sender }

func (Mail) Channel() string { return "mail" }

func (m Mail) Notify(event Event) error {
	if !strings.Contains(event.Who, "@") {
		return nil
	}
	subject := fmt.Sprintf("%s: %s", event.Title, event.State)
	var body strings.Builder
	fmt.Fprintf(&body, "The routine %q %s.\n", event.Title, map[string]string{"finished": "finished", "failed": "failed", "interrupted": "was stopped"}[event.State])
	if event.Answer != "" {
		body.WriteString("\n" + event.Answer + "\n")
	}
	if event.Link != "" {
		body.WriteString("\n" + event.Link + "\n")
	}
	return m.Via.Send(event.Who, subject, body.String())
}

// Every offers an event to each notifier that has not yet taken it. It gives back the channels that have, the ones that
// already had and the ones that just did, and an error naming those that did not.
type Every []Notifier

func (all Every) Offer(event Event, taken []string) ([]string, error) {
	now := append([]string{}, taken...)
	var failed []error
	for _, one := range all {
		if slices.Contains(taken, one.Channel()) {
			continue
		}
		if err := one.Notify(event); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", one.Channel(), err))
			continue
		}
		now = append(now, one.Channel())
	}
	return now, errors.Join(failed...)
}
