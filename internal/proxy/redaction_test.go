package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/redact"
	"github.com/getlatentic/gusto/internal/spend"
)

const (
	secretEmail = "ada.lovelace@analytical-engine.example.com"
	secretCard  = "4111 1111 1111 1111"
	secretKey   = "AKIAIOSFODNN7EXAMPLE"
)

// maskingSite is a proxy whose provider keeps what it was sent and answers with what it is told to.
type maskingSite struct {
	proxy    *Proxy
	token    string
	seen     string
	answer   string
	kind     string
	record   string
	provider *httptest.Server
}

func newMaskingSite(t *testing.T, wires []string) *maskingSite {
	t.Helper()
	a := &maskingSite{kind: "application/json", answer: `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`}
	a.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		a.seen = string(body)
		w.Header().Set("Content-Type", a.kind)
		io.WriteString(w, a.answer)
	}))
	t.Cleanup(a.provider.Close)
	tokens := NewTokens()
	kept := t.TempDir()
	ledger := spend.NewLedger(spend.Policy{})
	ledger.Begin("a-job", spend.Terms{})
	a.proxy = New(tokens, Credentials{"": {Name: "platform", Address: a.provider.URL, Key: "k", Wires: wires, MaxOutput: 4096}},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true }, ledger, Allowlist{})
	var err error
	if a.token, err = tokens.Mint("a-job", ""); err != nil {
		t.Fatal(err)
	}
	a.record = filepath.Join(kept, "a-job.jsonl")
	return a
}

func (a *maskingSite) turnOn(t *testing.T, kinds ...string) {
	t.Helper()
	redactor, err := redact.New(kinds...)
	if err != nil {
		t.Fatal(err)
	}
	a.proxy.UseRedaction(redactor)
}

func (a *maskingSite) ask(path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "/model/"+a.token+path, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	a.proxy.Handler().ServeHTTP(recorder, request)
	return recorder
}

func (a *maskingSite) lastLine(t *testing.T) spend.Call {
	t.Helper()
	data, err := os.ReadFile(a.record)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var call spend.Call
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &call); err != nil {
		t.Fatal(err)
	}
	return call
}

func chatAbout(text string) string {
	body, _ := json.Marshal(map[string]any{"model": "m", "messages": []map[string]string{{"role": "user", "content": text}}})
	return string(body)
}

func TestTheProviderSeesTheMaskAndNotAnyKindOfSecretInARequest(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.turnOn(t, redact.Names()...)
	text := "mail " + secretEmail + ", pay " + secretCard + ", " + secretKey
	if answer := a.ask("/v1/chat/completions", chatAbout(text)); answer.Code != http.StatusOK {
		t.Fatalf("answered %d", answer.Code)
	}
	for _, secret := range []string{secretEmail, secretCard, secretKey, "4111"} {
		if strings.Contains(a.seen, secret) {
			t.Errorf("the provider was sent %q: %s", secret, a.seen)
		}
	}
	for _, mask := range []string{"[redacted:email]", "[redacted:card]", "[redacted:key]"} {
		if !strings.Contains(a.seen, mask) {
			t.Errorf("the provider was not sent %s: %s", mask, a.seen)
		}
	}
	if !json.Valid([]byte(a.seen)) {
		t.Errorf("what the provider was sent is not JSON: %s", a.seen)
	}
}

func TestWhatWasMaskedIsCountedInTheRecordOfTheCallAndNotKept(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.turnOn(t, "email", "card", "key")
	a.answer = `{"choices":[{"message":{"content":"write to ` + secretEmail + `"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`
	a.ask("/v1/chat/completions", chatAbout(secretEmail+" and "+secretEmail+" and "+secretKey))
	call := a.lastLine(t)
	if call.Redacted["email"] != 2 || call.Redacted["key"] != 1 || call.RedactedAnswer["email"] != 1 {
		t.Errorf("redacted %v, answer %v", call.Redacted, call.RedactedAnswer)
	}
	data, _ := os.ReadFile(a.record)
	for _, secret := range []string{"ada.lovelace", secretKey} {
		if strings.Contains(string(data), secret) {
			t.Errorf("the record keeps %q", secret)
		}
	}
}

func TestAnAnswerThatHoldsASecretIsMaskedBeforeItReachesTheHarness(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.turnOn(t, "email")
	a.answer = `{"choices":[{"message":{"content":"the owner is ` + secretEmail + `"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`
	answer := a.ask("/v1/chat/completions", chatAbout("who owns it"))
	if strings.Contains(answer.Body.String(), secretEmail) || !strings.Contains(answer.Body.String(), "[redacted:email]") {
		t.Errorf("the harness was sent %s", answer.Body.String())
	}
	if answer.Header().Get("Content-Length") != "" {
		t.Error("the answer's length is the unmasked one")
	}
	if !json.Valid(answer.Body.Bytes()) {
		t.Error("the masked answer is not JSON")
	}
}

func TestAStreamedAnswerIsMaskedEventByEventAndEachEventArrivesWhenItIsSent(t *testing.T) {
	released := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"mail "+secretEmail+" now\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-released
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	tokens := NewTokens()
	proxy := New(tokens, Credentials{"": {Name: "platform", Address: provider.URL, Key: "k"}}, func(job string) (string, bool) { return filepath.Join(t.TempDir(), job), true }, spend.NewLedger(spend.Policy{}), Allowlist{})
	token, _ := tokens.Mint("a-job", "")
	redactor, _ := redact.New("email")
	proxy.UseRedaction(redactor)
	front := httptest.NewServer(proxy.Handler())
	defer front.Close()
	answer, err := http.Post(front.URL+"/model/"+token+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"m","stream":true,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	first := make([]byte, 512)
	n, _ := answer.Body.Read(first)
	if got := string(first[:n]); !strings.Contains(got, "[redacted:email]") || strings.Contains(got, "ada.lovelace") {
		t.Errorf("the first event was %q", got)
	}
	close(released)
	rest, _ := io.ReadAll(answer.Body)
	if !strings.Contains(string(rest), "[DONE]") {
		t.Errorf("the rest of the stream was %q", rest)
	}
}

func TestACallInAnotherShapeIsMaskedBeforeItIsTranslatedAndTheAnswerAfter(t *testing.T) {
	a := newMaskingSite(t, []string{"chat"})
	a.turnOn(t, "email")
	a.kind = "application/json"
	a.answer = `{"id":"1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"cc ` + secretEmail + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`
	messages := `{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hello ` + secretEmail + `"}]}`
	answer := a.ask("/v1/messages", messages)
	if strings.Contains(a.seen, secretEmail) || !strings.Contains(a.seen, "[redacted:email]") {
		t.Errorf("the provider was sent %s", a.seen)
	}
	if strings.Contains(answer.Body.String(), secretEmail) {
		t.Errorf("the harness was sent %s", answer.Body.String())
	}
	if call := a.lastLine(t); call.Redacted["email"] != 1 || call.RedactedAnswer["email"] != 1 || call.Translated != "messages" {
		t.Errorf("call = %+v", call)
	}
}

func TestWithNothingTurnedOnNothingIsMaskedAndTheRecordSaysNothing(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.ask("/v1/chat/completions", chatAbout(secretEmail))
	if !strings.Contains(a.seen, secretEmail) {
		t.Error("something was masked with redaction off")
	}
	if call := a.lastLine(t); call.Redacted != nil || call.RedactedAnswer != nil {
		t.Errorf("call = %+v", call)
	}
}

func TestRedactionTurnedOnWhileTheProxyRunsReachesTheNextCallAndNotTheOneBefore(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.ask("/v1/chat/completions", chatAbout(secretEmail))
	before := a.seen
	a.turnOn(t, "email")
	a.ask("/v1/chat/completions", chatAbout(secretEmail))
	if !strings.Contains(before, secretEmail) || strings.Contains(a.seen, secretEmail) {
		t.Errorf("before: %s\nafter: %s", before, a.seen)
	}
	a.proxy.UseRedaction(nil)
	a.ask("/v1/chat/completions", chatAbout(secretEmail))
	if !strings.Contains(a.seen, secretEmail) {
		t.Error("redaction stayed on after it was turned off")
	}
}

func TestAnAnswerThatIsNotTextIsPassedAsItWas(t *testing.T) {
	a := newMaskingSite(t, nil)
	a.turnOn(t, "email")
	a.kind, a.answer = "application/octet-stream", "bytes "+secretEmail
	if body := a.ask("/v1/chat/completions", chatAbout("x")).Body.String(); body != a.answer {
		t.Errorf("body = %q", body)
	}
}
