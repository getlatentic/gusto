package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/connector"
	"github.com/getlatentic/gusto/internal/connector/connectortest"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// An agent's call to an app its asker connected reaches the app with the asker's own token, through the proxy, and never with
// the token of whoever the task's files name: the asker is read from the task's state, which only this server writes.
func TestAnAgentCallsAnAppAsThePersonWhoAskedForTheTask(t *testing.T) {
	t.Setenv("GUSTO_GATEWAY_PRIVATE", "1")
	app := connectortest.New(t)
	entry := connector.Entry{ID: "tasks", Name: "Tasks", URL: app.MCP()}
	store, err := connector.Open(connector.Config{Dir: t.TempDir(), Base: "http://127.0.0.1", HTTP: http.DefaultClient, Apps: []connector.Entry{entry}})
	if err != nil {
		t.Fatal(err)
	}
	signIn, err := store.Begin(context.Background(), "ada@acme.example", entry)
	if err != nil {
		t.Fatal(err)
	}
	state, code := app.Approve(t, signIn)
	if _, err := store.Finish(context.Background(), "ada@acme.example", state, code); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	adas := connectedTask(t, root, "20261031-1200-ada", "ada@acme.example", app.MCP())
	bobs := connectedTask(t, root, "20261031-1300-bob", "bob@acme.example", app.MCP())
	gates, err := openGateways(root)
	if err != nil {
		t.Fatal(err)
	}
	gates.tokens = store
	tokens := proxy.NewTokens()
	where := func(id string) (string, bool) { return filepath.Join(root, id, "out", "proxy.jsonl"), true }
	p := proxy.New(tokens, proxy.Credentials{}, where, spend.NewLedger(spend.Policy{}), proxy.ParseAllowlist(""))
	p.UseGateways(gates.For)
	front := httptest.NewServer(p.Handler())
	t.Cleanup(front.Close)
	call := func(id string) string {
		token, err := tokens.Mint(id, "")
		if err != nil {
			t.Fatal(err)
		}
		answer := rpc(t, front.URL+"/model/mcp", token, "tools/call", map[string]any{"name": "tool_call",
			"arguments": map[string]any{"name": "tasks/list_issues", "arguments": map[string]any{}}})
		return fmt.Sprint(answer["result"], answer["error"])
	}

	if said := call(adas); !strings.Contains(said, "called as access-1") {
		t.Errorf("ada's agent was answered %s", said)
	}
	if said := call(bobs); strings.Contains(said, "called as") {
		t.Errorf("bob's agent, whose files name ada, reached the app: %s", said)
	}
	for _, token := range app.Called() {
		if token != "access-1" {
			t.Errorf("the app was called with %q, which is not ada's", token)
		}
	}
	for _, id := range []string{adas, bobs} {
		record, _ := os.ReadFile(filepath.Join(root, id, "out", "gateway.jsonl"))
		if strings.Contains(string(record), "access-") {
			t.Errorf("the record of %s holds a token: %s", id, record)
		}
	}
}

// connectedTask is a task asked for by who, whose gateway has the app behind it, and whose file names ada as the person, whoever
// asked.
func connectedTask(t *testing.T, root, id, who, address string) string {
	t.Helper()
	dir := job.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "x", Repo: "r", Harness: "fixture", Sensitivity: job.Internal, Budget: job.Budget{Turns: 1, Seconds: 60},
		Setup: &job.Setup{Gateway: []string{"tasks"}, Connectors: []string{"tasks"}}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]string{"id": id, "asked": who})
	os.WriteFile(filepath.Join(dir.Path(), "state.json"), state, 0o644)
	definitions, _ := json.Marshal([]map[string]any{{"name": "tasks", "url": address, "connector": "tasks", "person": "ada@acme.example"}})
	os.MkdirAll(dir.SetupDir(), 0o700)
	os.WriteFile(filepath.Join(dir.SetupDir(), setup.GatewayFile), definitions, 0o600)
	return id
}

// The operator's clients are read from Parameter Store when the setting names one, and a parameter that is not there, or not the
// clients, leaves the apps that need one off rather than stopping the server.
func TestConnectorClientsAreReadFromAParameterAndAMissingOneIsNone(t *testing.T) {
	stored := &storedParameters{held: map[string]string{
		"/gusto/poc/connector-clients": `{"github": {"id": "Ov23li", "secret": "shh"}}`,
		"/gusto/poc/not-clients":       `not json`,
	}}
	clients, err := connectorClients(context.Background(), "ssm:/gusto/poc/connector-clients", opening(stored))
	if err != nil || clients["github"].ID != "Ov23li" || clients["github"].Secret != "shh" {
		t.Errorf("the clients read were %v, %v", clients, err)
	}
	for _, setting := range []string{"ssm:/gusto/poc/missing", "ssm:/gusto/poc/not-clients"} {
		if clients, err := connectorClients(context.Background(), setting, opening(stored)); err != nil || clients != nil {
			t.Errorf("%s gave %v, %v", setting, clients, err)
		}
	}
	if clients, err := connectorClients(context.Background(), "", opening(stored)); err != nil || clients != nil {
		t.Errorf("no setting gave %v, %v", clients, err)
	}
}

// The front door signs people in with GitHub when the operator's clients include GitHub's, and offers no such sign-in otherwise.
func TestTheFrontDoorSignsInWithGitHubWhenItsClientIsStored(t *testing.T) {
	stored := &storedParameters{held: map[string]string{
		"/gusto/poc/connector-clients": `{"github": {"id": "Ov23li", "secret": "shh"}}`,
		"/gusto/poc/others":            `{"asana": {"id": "1", "secret": "2"}}`,
	}}
	github, err := githubSignIn(context.Background(), "ssm:/gusto/poc/connector-clients", opening(stored))
	if err != nil || github == nil || github.ID != "Ov23li" || github.Secret != "shh" {
		t.Errorf("GitHub's client gave %+v, %v", github, err)
	}
	for _, setting := range []string{"ssm:/gusto/poc/others", "ssm:/gusto/poc/missing", ""} {
		if github, err := githubSignIn(context.Background(), setting, opening(stored)); err != nil || github != nil {
			t.Errorf("%q gave %+v, %v", setting, github, err)
		}
	}
}
