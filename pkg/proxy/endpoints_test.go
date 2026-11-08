package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/wire"
)

// oneProviderTwoAddresses is DeepSeek as one credential: OpenAI's shapes at its root, Anthropic's under /anthropic.
func oneProviderTwoAddresses(t *testing.T) (*Proxy, string, *http.Header, *string) {
	t.Helper()
	var header http.Header
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, path = r.Header.Clone(), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	tokens := NewTokens()
	ledger := spend.NewLedger(fixturePrices)
	ledger.Begin("a-job", spend.Terms{})
	proxy := New(tokens, Credentials{"deepseek": {
		Name: "deepseek", Address: server.URL, Key: "the-real-key", Auth: Bearer,
		Endpoints: []Endpoint{{Address: server.URL + "/anthropic", Auth: Anthropic}},
	}}, func(string) (string, bool) { return "", false }, ledger, Allowlist{})
	token, err := tokens.Mint("a-job", "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	return proxy, token, &header, &path
}

func TestACallGoesToTheAddressOfItsProviderThatAnswersItsShape(t *testing.T) {
	proxy, token, header, path := oneProviderTwoAddresses(t)
	askAs(proxy, "/model/v1/messages", `{"model":"deepseek-flash","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`, map[string]string{"x-api-key": token})
	if *path != "/anthropic/v1/messages" || header.Get("x-api-key") != "the-real-key" || header.Get("Authorization") != "" {
		t.Fatalf("Messages went to %s with x-api-key %q and Authorization %q", *path, header.Get("x-api-key"), header.Get("Authorization"))
	}
	askAs(proxy, "/model/v1/chat/completions", `{"model":"deepseek-flash","messages":[{"role":"user","content":"hi"}]}`, map[string]string{"Authorization": "Bearer " + token})
	if *path != "/v1/chat/completions" || header.Get("Authorization") != "Bearer the-real-key" || header.Get("x-api-key") != "" {
		t.Fatalf("Chat Completions went to %s with Authorization %q and x-api-key %q", *path, header.Get("Authorization"), header.Get("x-api-key"))
	}
}

func TestAProviderWithAnAddressForEachShapeReachesEveryAgentNatively(t *testing.T) {
	proxy, _, _, _ := oneProviderTwoAddresses(t)
	for _, shape := range []wire.Shape{wire.Messages, wire.Chat, wire.Responses} {
		if native, ok := proxy.Reaches(context.Background(), "deepseek/deepseek-flash", shape); !native || !ok {
			t.Errorf("%s: native %v ok %v", shape, native, ok)
		}
	}
}

func TestACallInAShapeNoAddressAnswersIsTranslatedAtTheAddressThatAnswersChat(t *testing.T) {
	credential := Credential{Address: "https://a.example/anthropic", Auth: Anthropic, Key: "k",
		Endpoints: []Endpoint{{Address: "https://a.example/openai", Auth: Bearer, Wires: []string{"chat"}}}}
	at := credential.For(wire.Responses)
	if at.Address != "https://a.example/openai" || at.Auth != Bearer || at.Key != "k" || len(at.Endpoints) != 0 || !at.Translates(wire.Responses) {
		t.Fatalf("Responses went to %+v", at)
	}
}
