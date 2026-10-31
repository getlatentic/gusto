package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// signIns gives a person's token for an app, and none for anybody who did not sign in.
type signIns map[string]string

func (s signIns) Token(_ context.Context, person, connector string) (string, error) {
	if token, found := s[person+" "+connector]; found {
		return token, nil
	}
	return "", errors.New(person + " is not signed in to " + connector)
}

// An app a person connected is called with their own token, which the agent never sees; without a sign-in the call fails and says why.
func TestAnAppIsCalledWithTheTokenOfThePersonWhoConnectedIt(t *testing.T) {
	fake, address := newUpstream(t, tool("list_issues", "List the issues assigned to you."))
	fake.token = "adas-linear-token"
	h := start(t, Config{Servers: []Server{{Name: "linear", URL: address, Connector: "linear", Person: "ada@acme.example"}},
		Tokens: signIns{"ada@acme.example linear": "adas-linear-token"}})
	said, failed := h.call("tool_call", map[string]any{"name": "linear/list_issues"})
	if failed || !strings.HasPrefix(said, "did list_issues") {
		t.Fatalf("ada's call answered %q (failed %v)", said, failed)
	}

	stranger := start(t, Config{Servers: []Server{{Name: "linear", URL: address, Connector: "linear", Person: "bob@acme.example"}},
		Tokens: signIns{"ada@acme.example linear": "adas-linear-token"}})
	if found, _ := stranger.call("tool_search", map[string]any{"query": "issues"}); strings.Contains(found, "linear/list_issues:") {
		t.Errorf("bob, who never signed in, reached ada's linear: %s", found)
	}
}
