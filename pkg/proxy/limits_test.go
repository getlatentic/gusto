package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/spend"
)

func TestACallAsksForNoMoreOutputThanTheCapAndOneThatNamesNoneIsGivenIt(t *testing.T) {
	cases := []struct {
		path, body, field string
		want              int
	}{
		{"/v1/chat/completions", `{"model":"m","max_tokens":32000}`, "max_tokens", 4096},
		{"/v1/chat/completions", `{"model":"m","max_completion_tokens":9000}`, "max_completion_tokens", 4096},
		{"/v1/chat/completions", `{"model":"m","max_tokens":512}`, "max_tokens", 512},
		{"/v1/chat/completions", `{"model":"m"}`, "max_tokens", 4096},
		{"/v1/messages", `{"model":"m","max_tokens":64000}`, "max_tokens", 4096},
		{"/v1/responses", `{"model":"m"}`, "max_output_tokens", 4096},
		{"/v1/responses", `{"model":"m","max_output_tokens":0}`, "max_output_tokens", 4096},
	}
	for _, each := range cases {
		fields := map[string]any{}
		json.Unmarshal(capOutput([]byte(each.body), each.path, 4096), &fields)
		if got, _ := fields[each.field].(float64); int(got) != each.want {
			t.Errorf("%s %s: %s is %v, wanted %d", each.path, each.body, each.field, fields[each.field], each.want)
		}
	}
	if got := string(capOutput([]byte(`{"model":"m","max_tokens":32000}`), "/v1/chat/completions", 0)); got != `{"model":"m","max_tokens":32000}` {
		t.Errorf("with no cap the call changed: %s", got)
	}
	if got := string(capOutput([]byte(`not json`), "/v1/chat/completions", 4096)); got != "not json" {
		t.Errorf("what is not a call to a model changed: %s", got)
	}
}

// limitedSite is a proxy in front of a model that answers as the handler does, with the limits set.
func limitedSite(t *testing.T, limits Limits, model http.HandlerFunc) (*Proxy, string) {
	t.Helper()
	server := httptest.NewServer(model)
	t.Cleanup(server.Close)
	tokens := NewTokens()
	proxy := New(tokens, Credentials{"": {Address: server.URL, Key: "k", Name: "c"}},
		func(string) (string, bool) { return "", false }, spend.NewLedger(spend.Policy{}), Allowlist{})
	proxy.UseLimits(limits)
	token, _ := tokens.Mint("a-job", "")
	return proxy, token
}

func call(t *testing.T, proxy *Proxy, token, body string) (*httptest.ResponseRecorder, spend.Call, time.Duration) {
	t.Helper()
	ctx, result := WithResult(context.Background())
	request := httptest.NewRequest("POST", "/model/"+token+"/v1/chat/completions", strings.NewReader(body)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	began := time.Now()
	proxy.Handler().ServeHTTP(recorder, request)
	made, _ := result.Call()
	return recorder, made, time.Since(began)
}

// A stream that stops sending without ending is ended once it has been quiet for the limit, and the call says why.
func TestAStreamThatGoesQuietIsEnded(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	proxy, token := limitedSite(t, Limits{Silence: 200 * time.Millisecond}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	answer, made, took := call(t, proxy, token, `{"model":"m","stream":true}`)
	if took > 3*time.Second {
		t.Fatalf("a quiet stream held the call for %v", took)
	}
	if !strings.Contains(answer.Body.String(), "hi") || !strings.Contains(made.Error, "sent nothing") {
		t.Errorf("the call kept %q and says %q", answer.Body.String(), made.Error)
	}
}

// A stream that keeps sending is not ended, however long it takes in all.
func TestAStreamThatKeepsSendingIsNotEnded(t *testing.T) {
	proxy, token := limitedSite(t, Limits{Silence: 200 * time.Millisecond}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for range 8 {
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
		w.Write([]byte("data: [DONE]\n\n"))
	})
	answer, made, _ := call(t, proxy, token, `{"model":"m","stream":true}`)
	if made.Error != "" || !strings.Contains(answer.Body.String(), "[DONE]") {
		t.Errorf("a stream that kept sending was ended: %q", made.Error)
	}
}

// A stream the model never begins is answered as a gateway timeout, which an agent retries; an answer that is not streamed
// may take as long as it is written, since the cap on its output bounds it.
func TestAStreamThatNeverBeginsIsATimeoutAndAWholeAnswerIsWaitedFor(t *testing.T) {
	proxy, token := limitedSite(t, Limits{Silence: 200 * time.Millisecond}, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	})
	answer, made, _ := call(t, proxy, token, `{"model":"m","stream":true}`)
	if answer.Code != http.StatusGatewayTimeout || !strings.Contains(made.Error, "sent nothing") {
		t.Errorf("a stream that never began: %d %q", answer.Code, made.Error)
	}
	answer, made, _ = call(t, proxy, token, `{"model":"m"}`)
	if answer.Code != http.StatusOK || made.Error != "" || !strings.Contains(answer.Body.String(), "done") {
		t.Errorf("a whole answer was not waited for: %d %q", answer.Code, made.Error)
	}
}
