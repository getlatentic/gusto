// Package connectortest is an app with an MCP server that asks for OAuth, for tests of signing in to one.
package connectortest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// App names its resource metadata in a 401, registers clients, gives a code for a sign-in, and trades codes and refresh tokens
// for tokens that last as long as it is told. Its MCP server answers a call that carries a token it gave.
type App struct {
	Server *httptest.Server
	// Docs is whether it says it reads client ID metadata documents.
	Docs bool
	// SecretAuth is how it says a client's secret is sent, as token_endpoint_auth_methods_supported; none says nothing.
	SecretAuth []string

	mu      sync.Mutex
	codes   map[string]string // code to PKCE challenge
	lasts   time.Duration
	issued  int
	clients int
	tokens  map[string]bool
	called  []string
	scope   string
	secrets []string
}

// New is an app that runs until the test ends.
func New(t *testing.T) *App {
	t.Helper()
	a := &App{codes: map[string]string{}, lasts: time.Hour, tokens: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mcp", a.mcp)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"resource": a.MCP(), "authorization_servers": []string{a.Server.URL}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": a.Server.URL, "authorization_endpoint": a.Server.URL + "/authorize",
			"token_endpoint": a.Server.URL + "/token", "registration_endpoint": a.Server.URL + "/register",
			"code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": a.Docs,
			"token_endpoint_auth_methods_supported": a.SecretAuth})
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.clients++
		a.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"client_id": "registered-client"})
	})
	mux.HandleFunc("POST /token", a.token)
	a.Server = httptest.NewServer(mux)
	t.Cleanup(a.Server.Close)
	return a
}

// MCP is the address of the app's MCP server.
func (a *App) MCP() string { return a.Server.URL + "/mcp" }

// Lasts sets how long the tokens it gives from now on last.
func (a *App) Lasts(lasts time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lasts = lasts
}

// Clients is how many clients it has registered.
func (a *App) Clients() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.clients
}

// Secrets is how each call for tokens that carried a client's secret sent it: "basic <id> <secret>" or "form <id> <secret>".
func (a *App) Secrets() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.secrets...)
}

// Scope is what the last sign-in asked for.
func (a *App) Scope() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scope
}

// Called is the tokens its MCP server was called with, in order.
func (a *App) Called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.called...)
}

func (a *App) mcp(w http.ResponseWriter, r *http.Request) {
	token, bearer := cutBearer(r.Header.Get("Authorization"))
	a.mu.Lock()
	known := bearer && a.tokens[token]
	if known {
		a.called = append(a.called, token)
	}
	a.mu.Unlock()
	if !known {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+a.Server.URL+`/.well-known/oauth-protected-resource/mcp", scope="read write"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var message struct {
		ID     *int64 `json:"id"`
		Method string `json:"method"`
	}
	json.NewDecoder(r.Body).Decode(&message)
	if message.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result := map[string]any{}
	switch message.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "app", "version": "1"}}
	case "tools/list":
		result = map[string]any{"tools": []map[string]any{{"name": "list_issues", "description": "Lists your issues",
			"inputSchema": map[string]any{"type": "object"}}}}
	case "tools/call":
		result = map[string]any{"content": []map[string]any{{"type": "text", "text": "issue 1, called as " + token}}}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *message.ID, "result": result})
}

func (a *App) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	a.mu.Lock()
	defer a.mu.Unlock()
	if id, secret, basic := r.BasicAuth(); basic {
		a.secrets = append(a.secrets, "basic "+id+" "+secret)
	} else if secret := r.PostForm.Get("client_secret"); secret != "" {
		a.secrets = append(a.secrets, "form "+r.PostForm.Get("client_id")+" "+secret)
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		challenge, found := a.codes[r.Form.Get("code")]
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !found || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("resource") != a.MCP() {
			refuse(w)
			return
		}
		delete(a.codes, r.Form.Get("code"))
	case "refresh_token":
		if r.Form.Get("refresh_token") != "refresh-1" {
			refuse(w)
			return
		}
	}
	a.issued++
	access := fmt.Sprintf("access-%d", a.issued)
	a.tokens[access] = true
	json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "refresh-1",
		"expires_in": int(a.lasts.Seconds()), "token_type": "Bearer"})
}

func refuse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
}

func cutBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return "", false
	}
	return header[len(prefix):], true
}

// Approve is the person signing in at the app: it checks the sign-in address carries what it should, and gives a code for the
// challenge in it.
func (a *App) Approve(t *testing.T, signIn string) (state, code string) {
	t.Helper()
	parsed, err := url.Parse(signIn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("resource") != a.MCP() || query.Get("scope") == "" {
		t.Fatalf("the sign-in address lacks what it should carry: %s", signIn)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.scope = query.Get("scope")
	code = fmt.Sprintf("code-%d", len(a.codes)+1)
	a.codes[code] = query.Get("code_challenge")
	return query.Get("state"), code
}
