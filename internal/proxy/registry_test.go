package proxy

import (
	"context"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhatTheProxyHoldsCanBeChangedWhileItRunsAndACallIsWrittenWithTheVersion(t *testing.T) {
	proxy, tokens, kept, seen := site(t, "the-first-key")
	token, _ := tokens.Mint("a-job", "")
	path := "/model/" + token + "/v1/chat/completions"
	if ask(t, proxy, path, `{"model":"x"}`); seen.Get("Authorization") != "Bearer the-first-key" {
		t.Fatalf("%q", seen.Get("Authorization"))
	}
	held := Credentials{"": {Address: proxy.credentials()[""].Address, Key: "the-second-key", Name: "the-platform-key"}}
	proxy.SetCredentials(held, 7)
	if ask(t, proxy, path, `{"model":"x"}`); seen.Get("Authorization") != "Bearer the-second-key" {
		t.Errorf("a change is used by the next call: %q", seen.Get("Authorization"))
	}
	text, err := os.ReadFile(filepath.Join(kept, "a-job.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(text)), "\n")
	if len(lines) != 2 || strings.Contains(lines[0], `"registry"`) || !strings.Contains(lines[1], `"registry":7`) {
		t.Errorf("each call says which version of the registry it was sent under:\n%s", text)
	}
}

func TestAnAddressAnOrganisationChoseIsReachedOnlyAtAPublicAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"93.184.216.34":      true,
		"2606:2800:220:1::1": true,
		"127.0.0.1":          false,
		"::1":                false,
		"10.0.4.7":           false,
		"172.31.5.239":       false,
		"192.168.1.1":        false,
		"169.254.169.254":    false,
		"100.64.0.1":         false,
		"0.0.0.0":            false,
		"224.0.0.1":          false,
		"fd00::1":            false,
		"fe80::1":            false,
		"::ffff:10.0.0.1":    false,
	} {
		if got := Public(netip.MustParseAddr(address).Unmap()); got != want {
			t.Errorf("%s: %v, wanted %v", address, got, want)
		}
	}
}

func TestACredentialAnOrganisationAddedCannotReachThisMachineOrItsNetwork(t *testing.T) {
	proxy, tokens, _, seen := site(t, "k")
	model := proxy.credentials()[""].Address
	for name, resolve := range map[string]Resolver{
		"a name that resolves to this machine": func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		"a name that resolves to a private network": func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("172.31.5.239")}, nil
		},
		"a name that resolves to the metadata service": func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
		},
	} {
		proxy.resolve = resolve
		proxy.SetCredentials(Credentials{"": {Address: strings.Replace(model, "127.0.0.1", "model.example", 1), Key: "k", Name: "theirs", Public: true}}, 1)
		token, _ := tokens.Mint("job-"+strings.Fields(name)[len(strings.Fields(name))-1], "")
		answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)
		if answer.Code != http.StatusBadGateway || seen.Get("X-Seen-Path") != "" {
			t.Errorf("%s: %d, and the model was reached: %q", name, answer.Code, seen.Get("X-Seen-Path"))
		}
	}
	// And an address that is a literal of this machine is refused too, which is what a platform credential may have.
	proxy.SetCredentials(Credentials{"": {Address: model, Key: "k", Name: "theirs", Public: true}}, 1)
	token, _ := tokens.Mint("literal", "")
	if answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`); answer.Code != http.StatusBadGateway || seen.Get("X-Seen-Path") != "" {
		t.Errorf("a literal loopback address: %d", answer.Code)
	}
	// The same address is reached by a credential the operator set.
	proxy.SetCredentials(Credentials{"": {Address: model, Key: "k", Name: "ours"}}, 1)
	token, _ = tokens.Mint("operator", "")
	if answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`); answer.Code != http.StatusOK {
		t.Errorf("the operator's credential still reaches it: %d", answer.Code)
	}
}
