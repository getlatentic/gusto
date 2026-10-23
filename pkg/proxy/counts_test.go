package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getlatentic/gusto/pkg/spend"
)

func TestTheProxyCountsCallsAndSaysWhichFailedOnTheModelsSide(t *testing.T) {
	proxy, tokens, _, _ := site(t, "key")
	token, _ := tokens.Mint("a-job", "")
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)
	if got := proxy.Counts(); got != (Counts{Calls: 2}) {
		t.Fatalf("two good calls counted as %+v", got)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusBadGateway) }))
	defer failing.Close()
	broken := New(NewTokens(), Credentials{"": {Address: failing.URL, Key: "k", Name: "c"}},
		func(string) (string, bool) { return "", false }, spend.NewLedger(spend.Policy{}), Allowlist{})
	token, _ = broken.tokens.Mint("j", "")
	ask(t, broken, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)
	if got := broken.Counts(); got != (Counts{Calls: 1, Errors: 1}) {
		t.Fatalf("a 502 from the model counted as %+v", got)
	}
}

func TestOnlyTheModelsFailuresAreErrors(t *testing.T) {
	var c counters
	for _, call := range []spend.Call{
		{Status: 200},
		{Status: 400},
		{Status: 429},
		{Status: 500},
		{Status: 503},
		{Error: "connection refused"},
		{Refused: &spend.Refusal{Cap: "job", Reason: "spent"}},
	} {
		c.add(call)
	}
	if got := (Counts{c.calls.Load(), c.errors.Load(), c.refused.Load()}); got != (Counts{Calls: 7, Errors: 3, Refused: 1}) {
		t.Fatalf("counted %+v", got)
	}
}
