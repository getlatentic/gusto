package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAWebhookSendsTheEventSignedAndNeverTheAnswer(t *testing.T) {
	var got, signature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, signature = string(body), r.Header.Get("X-Gusto-Signature")
	}))
	defer server.Close()
	hook := Webhook{URL: server.URL, Secret: "s3cret", Client: server.Client()}
	if err := hook.Notify(Event{Name: "routine.finished", Title: "Morning", State: "finished", Answer: "private words", Job: "j1"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "private words") || !strings.Contains(got, `"job":"j1"`) {
		t.Errorf("sent %s", got)
	}
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(got))
	if signature != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Errorf("signature %q does not match the body", signature)
	}
}

func TestAWebhookThatAnswersBadlyIsAFailureToRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusBadGateway) }))
	defer server.Close()
	if err := (Webhook{URL: server.URL, Secret: "x", Client: server.Client()}).Notify(Event{}); err == nil {
		t.Error("a 502 was taken")
	}
}

func TestAWebhookAddressIsHTTPSAndHasASecret(t *testing.T) {
	for hook, ok := range map[Webhook]bool{
		{URL: "https://hooks.example/x", Secret: "s"}: true,
		{URL: "http://hooks.example/x", Secret: "s"}:  false,
		{URL: "https://hooks.example/x"}:              false,
		{URL: "not a url", Secret: "s"}:               false,
	} {
		if err := hook.Check(); (err == nil) != ok {
			t.Errorf("%+v: %v", hook, err)
		}
	}
}

type sent struct{ to, subject, body string }

func (s *sent) Send(to, subject, body string) error {
	s.to, s.subject, s.body = to, subject, body
	return nil
}

func TestMailGoesToWhoAddedTheRoutineWithTheAnswerAndTheLink(t *testing.T) {
	out := &sent{}
	err := Mail{Via: out}.Notify(Event{Title: "Morning", State: "failed", Who: "tosin@example.com", Answer: "it broke", Link: "https://gusto.example/jobs/j1"})
	if err != nil || out.to != "tosin@example.com" || out.subject != "Morning: failed" ||
		!strings.Contains(out.body, "it broke") || !strings.Contains(out.body, "https://gusto.example/jobs/j1") {
		t.Errorf("%v: %+v", err, out)
	}
	*out = sent{}
	if err := (Mail{Via: out}).Notify(Event{Title: "x", Who: "alice"}); err != nil || out.to != "" {
		t.Errorf("somebody who is not an address is not mailed: %v %+v", err, out)
	}
}

func TestEveryOffersAnEventOnlyToTheChannelsThatHaveNotTakenIt(t *testing.T) {
	mail, down := &sent{}, errors.New("down")
	all := Every{Mail{Via: mail}, failing{down}}
	taken, err := all.Offer(Event{Title: "t", State: "finished", Who: "a@b.c"}, nil)
	if mail.to == "" || !errors.Is(err, down) || len(taken) != 1 || taken[0] != "mail" {
		t.Fatalf("%v %v %+v", taken, err, mail)
	}
	*mail = sent{}
	taken, err = all.Offer(Event{Title: "t", State: "finished", Who: "a@b.c"}, taken)
	if mail.to != "" || !errors.Is(err, down) || len(taken) != 1 {
		t.Errorf("the mail that was taken was sent again: %v %v %+v", taken, err, mail)
	}
}

type failing struct{ err error }

func (failing) Channel() string      { return "webhook" }
func (f failing) Notify(Event) error { return f.err }
