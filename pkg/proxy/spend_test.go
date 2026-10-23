package proxy

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/spend"
)

// These answers are fixtures written to the documented OpenAI-compatible shape,
// not observed from a real provider. The path the proxy reads is not yet
// measured against a real upstream.
const (
	plainAnswer = `{"choices":[{"message":{"content":"yes"}}],"usage":{"prompt_tokens":1000000,"completion_tokens":100000}}`
	streamed    = "data: {\"choices\":[{\"delta\":{\"content\":\"yes\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":500000,\"completion_tokens\":0}}\n\n" +
		"data: [DONE]\n\n"
)

// fixturePrices are made up for the test and are nobody's real price.
var fixturePrices = spend.Policy{
	Prices: map[string]spend.Price{"fixture-model": {InputPerMillion: 2, OutputPerMillion: 10}},
}

// model answers every call the same way and counts the calls it was sent.
type model struct {
	contentType string
	answer      string
	calls       int
	lastBody    string
	gzipped     bool
}

func (m *model) serve(w http.ResponseWriter, r *http.Request) {
	m.calls++
	body, _ := io.ReadAll(r.Body)
	m.lastBody = string(body)
	w.Header().Set("Content-Type", m.contentType)
	if m.gzipped && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		packed := gzip.NewWriter(w)
		packed.Write([]byte(m.answer))
		packed.Close()
		return
	}
	w.Write([]byte(m.answer))
}

func counted(t *testing.T, answering *model, terms spend.Terms) (*Proxy, string, string, *spend.Ledger) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(answering.serve))
	t.Cleanup(server.Close)
	tokens := NewTokens()
	kept := t.TempDir()
	ledger := spend.NewLedger(fixturePrices)
	ledger.Begin("a-job", terms)
	proxy := New(tokens, Credentials{"": {Address: server.URL, Name: "the-platform-key"}},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true }, ledger, Allowlist{})
	token, err := tokens.Mint("a-job", "")
	if err != nil {
		t.Fatal(err)
	}
	return proxy, token, filepath.Join(kept, "a-job.jsonl"), ledger
}

func calls(t *testing.T, path string) []spend.Call {
	t.Helper()
	found, err := spend.ReadCalls(path)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestWhatACallCostIsCountedFromTheProvidersAnswer(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"fixture-model"}`)
	if answer.Body.String() != plainAnswer {
		t.Fatalf("the container was sent %q", answer.Body.String())
	}
	seen := calls(t, record)[0]
	if seen.Model != "fixture-model" || *seen.InputTokens != 1_000_000 || *seen.OutputTokens != 100_000 {
		t.Fatalf("the record is %+v", seen)
	}
	if seen.USD == nil || *seen.USD != 3 {
		t.Fatalf("the cost is %v", seen.USD)
	}
}

// A stream only carries usage when it is asked to, so the proxy asks.
func TestAStreamedCallIsAskedForItsUsageAndCounted(t *testing.T) {
	answering := &model{contentType: "text/event-stream", answer: streamed}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	ask(t, proxy, "/model/"+token+"/v1/chat/completions",
		`{"model":"fixture-model","stream":true,"stream_options":{"something":1},"messages":[{"role":"user","content":"hi"}]}`)

	var sent map[string]any
	if err := json.Unmarshal([]byte(answering.lastBody), &sent); err != nil {
		t.Fatal(err)
	}
	options, _ := sent["stream_options"].(map[string]any)
	if options["include_usage"] != true || options["something"] != float64(1) || sent["messages"] == nil {
		t.Fatalf("the model was sent %s", answering.lastBody)
	}
	seen := calls(t, record)[0]
	if seen.USD == nil || *seen.USD != 1 {
		t.Fatalf("the record is %+v", seen)
	}
}

func TestAnAnswerWithoutUsageIsRecordedAsNullNotZero(t *testing.T) {
	answering := &model{contentType: "application/json", answer: `{"choices":[]}`}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"fixture-model"}`)
	text, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"inputTokens":null`, `"outputTokens":null`, `"usd":null`} {
		if !strings.Contains(string(text), field) {
			t.Fatalf("want %s in %s", field, text)
		}
	}
}

// Go unpacks gzip only when it asked for it, so a compression the container
// chose would reach the meter packed and go uncounted.
func TestACompressedAnswerIsStillCounted(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer, gzipped: true}
	proxy, token, record, _ := counted(t, answering, spend.Terms{})
	request := httptest.NewRequest("POST", "/model/"+token+"/v1/chat/completions", strings.NewReader(`{"model":"fixture-model"}`))
	request.Header.Set("Accept-Encoding", "br")
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	if recorder.Body.String() != plainAnswer {
		t.Fatalf("the container was sent %q", recorder.Body.String())
	}
	if seen := calls(t, record)[0]; seen.USD == nil {
		t.Fatalf("a compressed answer went uncounted: %+v", seen)
	}
}

// Once the cap is spent the next call does not leave, and the harness is told
// why in the shape its client already prints.
func TestASpentCapRefusesTheNextCallBeforeItLeaves(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	limit := 2.0
	proxy, token, record, _ := counted(t, answering, spend.Terms{CapUSD: &limit})
	if answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"fixture-model"}`); answer.Code != 200 {
		t.Fatalf("the first call was refused: %d", answer.Code)
	}
	answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"fixture-model"}`)
	if answer.Code != http.StatusPaymentRequired || !strings.Contains(answer.Body.String(), "this task has spent $3.0000 of its $2.0000 cap") {
		t.Fatalf("code %d: %s", answer.Code, answer.Body.String())
	}
	if answering.calls != 1 {
		t.Fatalf("the model was sent %d calls", answering.calls)
	}
	refused := calls(t, record)[1]
	if refused.Refused == nil || refused.Refused.Cap != "job" || *refused.USD != 0 {
		t.Fatalf("the refusal was recorded as %+v", refused)
	}
}

func TestAModelWithNoPriceIsRefusedUnderACap(t *testing.T) {
	answering := &model{contentType: "application/json", answer: plainAnswer}
	limit := 2.0
	proxy, token, _, _ := counted(t, answering, spend.Terms{CapUSD: &limit})
	answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"another-model"}`)
	if answer.Code != http.StatusPaymentRequired || answering.calls != 0 {
		t.Fatalf("code %d, and the model was sent %d calls", answer.Code, answering.calls)
	}
}
