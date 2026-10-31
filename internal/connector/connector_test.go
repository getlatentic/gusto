package connector

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// app stands in for an app with an MCP server that asks for OAuth: it names its resource metadata in a 401, registers clients,
// gives a code for a sign-in, and trades codes and refresh tokens for tokens that last as long as it is told.
type app struct {
	server  *httptest.Server
	mu      sync.Mutex
	codes   map[string]string // code to PKCE challenge
	lasts   time.Duration
	issued  int
	clients int
	docs    bool
}

func newApp(t *testing.T) *app {
	t.Helper()
	a := &app{codes: map[string]string{}, lasts: time.Hour}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.server.URL+`/.well-known/oauth-protected-resource/mcp", scope="read write"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"resource": a.server.URL + "/mcp", "authorization_servers": []string{a.server.URL}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": a.server.URL, "authorization_endpoint": a.server.URL + "/authorize",
			"token_endpoint": a.server.URL + "/token", "registration_endpoint": a.server.URL + "/register",
			"code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": a.docs})
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.clients++
		a.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "registered-client"})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		a.mu.Lock()
		defer a.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			challenge, found := a.codes[r.Form.Get("code")]
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !found || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("resource") != a.server.URL+"/mcp" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			delete(a.codes, r.Form.Get("code"))
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
		}
		a.issued++
		json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("access-%d", a.issued), "refresh_token": "refresh-1",
			"expires_in": int(a.lasts.Seconds()), "token_type": "Bearer"})
	})
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}

// approve is the person signing in at the app: the app gives a code for the challenge the sign-in address carried.
func (a *app) approve(t *testing.T, signIn string) (state, code string) {
	t.Helper()
	parsed, err := url.Parse(signIn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("resource") != a.server.URL+"/mcp" || query.Get("scope") != "read write" {
		t.Fatalf("the sign-in address lacks what it should carry: %s", signIn)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	code = fmt.Sprintf("code-%d", len(a.codes)+1)
	a.codes[code] = query.Get("code_challenge")
	return query.Get("state"), code
}

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
	a := newApp(t)
	c := connectors(t, "https://acme.gusto.test")
	entry := Entry{ID: "tasks", Name: "Tasks", URL: a.server.URL + "/mcp"}
	signIn, err := c.Begin(context.Background(), "Ada@Acme.com", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signIn, "client_id=registered-client") || !strings.Contains(signIn, url.QueryEscape("https://acme.gusto.test/connections/callback")) {
		t.Errorf("the sign-in is not as the registered client, back to this server: %s", signIn)
	}
	state, code := a.approve(t, signIn)
	if _, err := c.Finish(context.Background(), "bob@acme.com", state, code); err == nil {
		t.Error("somebody else finished ada's sign-in")
	}
	state, code = a.approve(t, mustBegin(t, c, "ada@acme.com", entry))
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
	if a.clients != 1 {
		t.Errorf("gusto registered %d times, not once", a.clients)
	}

	a.lasts = 30 * time.Second
	state, code = a.approve(t, mustBegin(t, c, "ada@acme.com", entry))
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
	a := newApp(t)
	a.docs = true
	c := connectors(t, "https://acme.gusto.test")
	signIn := mustBegin(t, c, "ada@acme.com", Entry{ID: "tasks", URL: a.server.URL + "/mcp"})
	if !strings.Contains(signIn, url.QueryEscape("https://acme.gusto.test/connectors/client.json")) || a.clients != 0 {
		t.Errorf("the client is not the document: %s, %d registered", signIn, a.clients)
	}
	if document := c.ClientDocument(); document["client_id"] != "https://acme.gusto.test/connectors/client.json" {
		t.Errorf("the document names %v", document["client_id"])
	}
	operator, _ := Open(Config{Dir: t.TempDir(), Base: "https://acme.gusto.test", HTTP: http.DefaultClient, Clients: map[string]Client{"tasks": {ID: "operators-app"}}})
	if signIn := mustBegin(t, operator, "ada@acme.com", Entry{ID: "tasks", URL: a.server.URL + "/mcp"}); !strings.Contains(signIn, "client_id=operators-app") {
		t.Errorf("the operator's client was not used: %s", signIn)
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
