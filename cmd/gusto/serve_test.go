package main

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// A job naming a credential this deployment does not hold is refused by name,
// before any box starts — not as a model call that fails somewhere inside
// a run somebody then has to go and read.
func TestAJobNamingACredentialNobodyHoldsIsRefusedBeforeItStarts(t *testing.T) {
	way := &WayOut{
		proxy:  &proxy.Proxy{},
		tokens: proxy.NewTokens(),
		ledger: spend.NewLedger(spend.Policy{}),
	}
	held := proxy.Credentials{"platform": {Name: "platform", Address: "https://example.invalid"}}
	way.held.Store(&held)
	dir := job.Dir(t.TempDir())
	_, err := way.Token(dir, job.Spec{Model: "nowhere/some-model"})
	if err == nil {
		t.Fatal("a credential nobody holds was given a token")
	}
	for _, want := range []string{"nowhere", "platform"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}
