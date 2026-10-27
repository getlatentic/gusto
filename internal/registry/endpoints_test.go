package registry

import (
	"slices"
	"testing"

	"github.com/getlatentic/gusto/pkg/proxy"
)

func TestAProviderGetsAnotherAddressThatIsCheckedLikeItsFirstAndCanLoseIt(t *testing.T) {
	r, _ := opened(t, Rules{})
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []proxy.Endpoint{
		{Address: "https://inside.example/anthropic", Auth: proxy.Anthropic},
		{Address: "https://api.theirs.example/v1"},
		{Address: "https://api.theirs.example/x", Wires: []string{"grpc"}},
	} {
		if err := r.AddAddress("acme", refused); err == nil {
			t.Errorf("%+v was kept", refused)
		}
	}
	if err := r.AddAddress("nobody", proxy.Endpoint{Address: "https://api.theirs.example/anthropic"}); err != ErrNoSuch {
		t.Errorf("a provider that is not there: %v", err)
	}
	if err := r.AddAddress("acme", proxy.Endpoint{Address: "https://api.theirs.example/anthropic/", Auth: proxy.Anthropic}); err != nil {
		t.Fatal(err)
	}
	r.Enable("acme", "")
	held := r.Effective().Credentials["acme"]
	if len(held.Endpoints) != 1 || held.Endpoints[0].Address != "https://api.theirs.example/anthropic" || held.Key != "sk-their-secret-key" {
		t.Fatalf("the proxy holds %+v", held)
	}
	if err := r.RemoveAddress("acme", "https://api.theirs.example/v1"); err != ErrNoSuch {
		t.Errorf("the first address went without its provider: %v", err)
	}
	if err := r.RemoveAddress("acme", "https://api.theirs.example/anthropic"); err != nil {
		t.Fatal(err)
	}
	if len(r.Effective().Credentials["acme"].Endpoints) != 0 {
		t.Error("the address is still held")
	}
}

func TestAnAddedAddressIsSaidAsAChangeToItsProvider(t *testing.T) {
	was := Document{Providers: []Provider{{Name: "acme", Address: "https://a.example"}}}
	now := Document{Providers: []Provider{{Name: "acme", Address: "https://a.example", Endpoints: []proxy.Endpoint{{Address: "https://a.example/anthropic"}}}}}
	if lines := Describe(was, now); !slices.Equal(lines, []string{"changed provider acme"}) {
		t.Errorf("Describe says %v", lines)
	}
}

func TestEachKindIsSaidInTheWordsItWasChosenBy(t *testing.T) {
	for _, each := range []struct {
		auth  proxy.Auth
		wires []string
		want  string
	}{
		{proxy.Bearer, nil, "OpenAI-compatible"},
		{"", nil, "OpenAI-compatible"},
		{proxy.Anthropic, nil, "Anthropic-compatible"},
		{proxy.Bearer, []string{"chat"}, "custom: chat, bearer token"},
	} {
		if got := KindOf(each.auth, each.wires); got != each.want {
			t.Errorf("%s %v: %q", each.auth, each.wires, got)
		}
	}
}
