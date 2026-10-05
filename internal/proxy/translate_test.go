package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/spend"
)

// chatProvider answers Chat Completions alone, as the Gemma models on Bedrock do, and keeps what it was sent.
type chatProvider struct {
	path, body string
	header     http.Header
}

const chatProviderStream = "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000000,\"completion_tokens\":100000}}\n\n" +
	"data: [DONE]\n\n"

func (c *chatProvider) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.path, c.body, c.header = r.URL.Path, string(body), r.Header.Clone()
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.Error(w, "this provider answers Chat Completions only", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	io.WriteString(w, chatProviderStream)
}

// translating is a proxy whose one credential reaches a provider that answers Chat Completions alone and says so.
func translating(t *testing.T, wires []string) (*Proxy, string, string, *chatProvider) {
	t.Helper()
	provider := &chatProvider{}
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	t.Cleanup(server.Close)
	tokens := NewTokens()
	kept := t.TempDir()
	ledger := spend.NewLedger(fixturePrices)
	ledger.Begin("a-job", spend.Terms{})
	proxy := New(tokens, Credentials{"gemma": {Name: "gemma", Address: server.URL + "/openai", Key: "the-real-key", Wires: wires, MaxOutput: 8192}},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true }, ledger, Allowlist{})
	token, err := tokens.Mint("a-job", "gemma")
	if err != nil {
		t.Fatal(err)
	}
	return proxy, token, filepath.Join(kept, "a-job.jsonl"), provider
}

func askAs(proxy *Proxy, path, body string, header map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", path, strings.NewReader(body))
	for name, value := range header {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestClaudeCodesCallIsTranslatedForAProviderThatAnswersChatCompletionsAloneAndCountedTheSame(t *testing.T) {
	proxy, token, record, provider := translating(t, []string{"chat"})
	asked := `{"model":"fixture-model","max_tokens":32000,"stream":true,"system":[{"type":"text","text":"be brief"}],
		"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"adaptive"}}`
	answer := askAs(proxy, "/model/v1/messages", asked, map[string]string{"x-api-key": token, "anthropic-version": "2023-06-01", "anthropic-beta": "claude-code-20250219"})

	if provider.path != "/openai/v1/chat/completions" {
		t.Fatalf("the provider was asked for %q", provider.path)
	}
	if provider.header.Get("Authorization") != "Bearer the-real-key" || provider.header.Get("x-api-key") != "" || provider.header.Get("anthropic-beta") != "" || provider.header.Get("anthropic-version") != "" {
		t.Errorf("the provider was sent %v", provider.header)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(provider.body), &sent); err != nil {
		t.Fatal(err)
	}
	if _, found := sent["messages"]; !found || string(sent["max_tokens"]) != "8192" || sent["thinking"] != nil || sent["system"] != nil {
		t.Errorf("the provider was sent %s", provider.body)
	}
	if answer.Code != 200 || answer.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(answer.Body.String(), "event: message_start") ||
		!strings.Contains(answer.Body.String(), `"text":"done"`) || !strings.Contains(answer.Body.String(), `"stop_reason":"end_turn"`) {
		t.Errorf("Claude Code was sent %d %s", answer.Code, answer.Body)
	}
	seen := calls(t, record)[0]
	if seen.Translated != "messages" || seen.Path != "/v1/messages" || seen.Model != "fixture-model" || *seen.InputTokens != 1_000_000 || seen.USD == nil || *seen.USD != 3 {
		t.Errorf("the record is %+v", seen)
	}
}

func TestCodexsCallIsTranslatedTheSameWay(t *testing.T) {
	proxy, token, record, provider := translating(t, []string{"chat"})
	asked := `{"model":"fixture-model","stream":true,"instructions":"be brief","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"store":false}`
	answer := askAs(proxy, "/model/v1/responses", asked, map[string]string{"Authorization": "Bearer " + token})
	if provider.path != "/openai/v1/chat/completions" || !strings.Contains(answer.Body.String(), "event: response.completed") {
		t.Errorf("%s: %d %s", provider.path, answer.Code, answer.Body)
	}
	if seen := calls(t, record)[0]; seen.Translated != "responses" || *seen.OutputTokens != 100_000 {
		t.Errorf("the record is %+v", seen)
	}
}

// A credential that names no wires is a provider that answers what its authentication says, and a call goes to it as
// it came, as it always has: translation is something a deployment turns on for a provider it has measured.
func TestACredentialThatNamesNoWiresIsSentEveryCallAsItCame(t *testing.T) {
	proxy, token, record, provider := translating(t, nil)
	answer := askAs(proxy, "/model/v1/messages", `{"model":"fixture-model","messages":[]}`, map[string]string{"x-api-key": token})
	if provider.path != "/openai/v1/messages" || answer.Code != http.StatusNotFound {
		t.Errorf("%s: %d", provider.path, answer.Code)
	}
	if seen := calls(t, record)[0]; seen.Translated != "" {
		t.Errorf("the record is %+v", seen)
	}
}

// A call in Chat Completions to a provider that answers it is never touched, wires or not.
func TestACallInTheShapeTheProviderAnswersGoesAsItCame(t *testing.T) {
	proxy, token, _, provider := translating(t, []string{"chat"})
	asked := `{"model":"fixture-model","messages":[{"role":"user","content":"hi"}],"stream":true}`
	answer := askAs(proxy, "/model/v1/chat/completions", asked, map[string]string{"Authorization": "Bearer " + token})
	if answer.Body.String() != chatProviderStream || !strings.Contains(provider.body, `"include_usage":true`) {
		t.Errorf("%s / %s", answer.Body, provider.body)
	}
}

func TestACallThatCannotBeTranslatedIsRefusedInTheHarnesssShapeAndNeverSent(t *testing.T) {
	proxy, token, _, provider := translating(t, []string{"chat"})
	answer := askAs(proxy, "/model/v1/messages", `{"model":"fixture-model","messages":[{"role":"user","content":7}]}`, map[string]string{"x-api-key": token})
	if answer.Code != http.StatusBadRequest || !strings.Contains(answer.Body.String(), `"type":"error"`) || provider.path != "" {
		t.Errorf("%d %s, provider asked for %q", answer.Code, answer.Body, provider.path)
	}
}
