package connector

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/connector/connectortest"
)

func connectors(t *testing.T, base string) *Connectors {
	t.Helper()
	c, err := Open(Config{Dir: t.TempDir(), Base: base, HTTP: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A person signs in at the app, comes back, and their agents' calls get a token the app gave them, renewed when it runs out.
func TestAPersonSignsInAndTheirTokenIsKeptAndRenewed(t *testing.T) {
	a := connectortest.New(t)
	c := connectors(t, "https://acme.gusto.test")
	entry := Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}
	signIn, err := c.Begin(context.Background(), "Ada@Acme.com", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signIn, "client_id=registered-client") || !strings.Contains(signIn, url.QueryEscape("https://acme.gusto.test/connections/callback")) {
		t.Errorf("the sign-in is not as the registered client, back to this server: %s", signIn)
	}
	state, code := a.Approve(t, signIn)
	if _, err := c.Finish(context.Background(), "bob@acme.com", state, code); err == nil {
		t.Error("somebody else finished ada's sign-in")
	}
	state, code = a.Approve(t, mustBegin(t, c, "ada@acme.com", entry))
	if connector, err := c.Finish(context.Background(), "ada@acme.com", state, code); err != nil || connector != "tasks" {
		t.Fatalf("finishing: %q %v", connector, err)
	}
	if _, err := c.Finish(context.Background(), "ada@acme.com", state, code); err == nil {
		t.Error("a sign-in was finished twice")
	}
	if token, err := c.Token(context.Background(), "ada@acme.com", "tasks"); err != nil || token != "access-1" {
		t.Errorf("ada's token: %q %v", token, err)
	}
	if _, connected := c.Connected("ADA@acme.com", "tasks"); !connected {
		t.Error("ada is not connected")
	}
	if _, err := c.Token(context.Background(), "bob@acme.com", "tasks"); err == nil {
		t.Error("bob, who never signed in, has a token")
	}
	if a.Clients() != 1 {
		t.Errorf("gusto registered %d times, not once", a.Clients())
	}

	a.Lasts(30 * time.Second)
	state, code = a.Approve(t, mustBegin(t, c, "ada@acme.com", entry))
	c.Finish(context.Background(), "ada@acme.com", state, code)
	if token, err := c.Token(context.Background(), "ada@acme.com", "tasks"); err != nil || token != "access-3" {
		t.Errorf("a token about to run out was not renewed: %q %v", token, err)
	}
	if err := c.Disconnect("ada@acme.com", "tasks"); err != nil {
		t.Fatal(err)
	}
	if _, connected := c.Connected("ada@acme.com", "tasks"); connected {
		t.Error("ada is connected after disconnecting")
	}
}

// An app that reads client documents knows this server by its document, and registers nothing; one that can do neither is said.
func TestAClientIsTheDocumentOrTheOperatorsWhenTheAppRegistersNone(t *testing.T) {
	a := connectortest.New(t)
	a.Docs = true
	c := connectors(t, "https://acme.gusto.test")
	signIn := mustBegin(t, c, "ada@acme.com", Entry{ID: "tasks", URL: a.MCP()})
	if !strings.Contains(signIn, url.QueryEscape("https://acme.gusto.test/connectors/client.json")) || a.Clients() != 0 {
		t.Errorf("the client is not the document: %s, %d registered", signIn, a.Clients())
	}
	if document := c.ClientDocument(); document["client_id"] != "https://acme.gusto.test/connectors/client.json" {
		t.Errorf("the document names %v", document["client_id"])
	}
	tasks := Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}
	operator, err := Open(Config{Dir: t.TempDir(), Base: "https://acme.gusto.test", HTTP: http.DefaultClient, Apps: []Entry{tasks},
		Clients: map[string]Client{"tasks": {ID: "operators-app"}}})
	if err != nil {
		t.Fatal(err)
	}
	if signIn := mustBegin(t, operator, "ada@acme.com", tasks); !strings.Contains(signIn, "client_id=operators-app") {
		t.Errorf("the operator's client was not used: %s", signIn)
	}
}

// A client for an app that is not offered is a mistake in the operator's file, said when the server starts.
func TestAClientForNoConnectorIsRefused(t *testing.T) {
	if _, err := Open(Config{Dir: t.TempDir(), Base: "https://acme.gusto.test", Clients: map[string]Client{"nothing": {ID: "x"}}}); err == nil {
		t.Error("a client for no connector was taken")
	}
}

// An operator adds their own apps beside the catalog's, each with a name and an https address.
func TestMoreAppsAreCheckedAndAdded(t *testing.T) {
	all, err := WithMore([]Entry{{ID: "wiki", Name: "Wiki", URL: "https://wiki.acme.com/mcp"}})
	if err != nil || len(all) != len(Catalog)+1 || all[len(all)-1].ID != "wiki" {
		t.Fatalf("the app was not added: %v %v", all, err)
	}
	for _, bad := range []Entry{{ID: "linear", Name: "Again", URL: "https://x.com"}, {ID: "Wiki", Name: "W", URL: "https://x.com"},
		{ID: "wiki", Name: "W", URL: "http://wiki.acme.com"}, {ID: "wiki", URL: "https://x.com"}} {
		if _, err := WithMore([]Entry{bad}); err == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
}

func mustBegin(t *testing.T, c *Connectors, person string, entry Entry) string {
	t.Helper()
	signIn, err := c.Begin(context.Background(), person, entry)
	if err != nil {
		t.Fatal(err)
	}
	return signIn
}

// What is kept opens only with the folder's own key, and only under the name it was kept at.
func TestTokensAreSealed(t *testing.T) {
	dir := t.TempDir()
	v, err := openVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	v.put("tokens/a.json", map[string]string{"access": "secret-token"})
	other, _ := openVault(t.TempDir())
	other.dir = dir
	var got map[string]string
	if _, err := other.get("tokens/a.json", &got); err == nil {
		t.Error("tokens opened with another key")
	}
	if found, err := v.get("tokens/a.json", &got); !found || err != nil || got["access"] != "secret-token" {
		t.Errorf("%v %v %v", found, err, got)
	}
}
