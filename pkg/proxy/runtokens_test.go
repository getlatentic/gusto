package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/runtoken"
	"github.com/getlatentic/gusto/pkg/spend"
)

// aBoundRun is a proxy that lets in a run whose sandbox holds a key, the model behind it, and the run's secret.
func aBoundRun(t *testing.T) (*httptest.Server, *Tokens, string, *string) {
	t.Helper()
	var seen string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(model.Close)
	tokens := NewTokens()
	ledger := spend.NewLedger(spend.Policy{})
	p := New(tokens, Credentials{"": {Address: model.URL, Key: "the-real-key"}}, func(string) (string, bool) { return "", false }, ledger, Allowlist{})
	issuer, err := runtoken.NewIssuer(runtoken.TokenLife(), func(string) runtoken.Run { return runtoken.Run{Person: "ada@acme.com", Agent: "a1"} })
	if err != nil {
		t.Fatal(err)
	}
	p.UseRunTokens(issuer)
	server := httptest.NewServer(p.Handler())
	t.Cleanup(server.Close)
	secret, err := tokens.MintBound("a-job", "")
	if err != nil {
		t.Fatal(err)
	}
	ledger.Begin("a-job", spend.Terms{})
	return server, tokens, secret, &seen
}

func post(t *testing.T, url string, header map[string]string) int {
	t.Helper()
	request, _ := http.NewRequest("POST", url, strings.NewReader(`{"model":"m","messages":[]}`))
	for name, value := range header {
		request.Header.Set(name, value)
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	return answer.StatusCode
}

func TestAnAgentReachesItsModelThroughItsForwarderAndNothingItHoldsWorksElsewhere(t *testing.T) {
	proxy, tokens, secret, seen := aBoundRun(t)
	if code := post(t, proxy.URL+"/model/v1/chat/completions", map[string]string{"Authorization": "Bearer " + secret}); code != http.StatusForbidden {
		t.Fatalf("the secret worked as a token: %d", code)
	}
	forwarder, err := runtoken.NewForwarder(proxy.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(forwarder)
	defer local.Close()
	if code := post(t, local.URL+"/model/v1/chat/completions", map[string]string{"Authorization": "Bearer what-the-agent-says"}); code != http.StatusOK {
		t.Fatalf("the agent's call through its forwarder gave %d", code)
	}
	if *seen != "Bearer the-real-key" {
		t.Errorf("the model was sent %q", *seen)
	}
	if _, err := runtoken.NewForwarder(proxy.URL, secret); err == nil {
		t.Error("the secret was traded a second time")
	}
	tokens.Drop("a-job")
	if code := post(t, local.URL+"/model/v1/chat/completions", nil); code != http.StatusForbidden {
		t.Errorf("a call after the run ended gave %d", code)
	}
}

func TestACallCopiedFromTheWireIsRefusedTheSecondTime(t *testing.T) {
	proxy, _, secret, _ := aBoundRun(t)
	target, _ := url.Parse(proxy.URL)
	relay := httputil.NewSingleHostReverseProxy(target)
	var copied http.Header
	tap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		copied = r.Header.Clone()
		relay.ServeHTTP(w, r)
	}))
	defer tap.Close()
	forwarder, err := runtoken.NewForwarder(tap.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(forwarder)
	defer local.Close()
	if code := post(t, local.URL+"/model/v1/chat/completions", nil); code != http.StatusOK {
		t.Fatalf("the call gave %d", code)
	}
	request, _ := http.NewRequest("POST", proxy.URL+"/model/v1/chat/completions", strings.NewReader("{}"))
	request.Host = strings.TrimPrefix(tap.URL, "http://")
	request.Header.Set("Authorization", copied.Get("Authorization"))
	request.Header.Set("DPoP", copied.Get("DPoP"))
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.StatusCode != http.StatusForbidden {
		t.Errorf("the same call sent again gave %d", answer.StatusCode)
	}
	if _, err := runtoken.NewForwarder(proxy.URL, "never-given"); err == nil {
		t.Error("a secret nobody gave was traded")
	}
}
