package web

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"slices"
	"testing"
)

// docs/PROTOCOL.md names every call of the protocol, and only them, so the document cannot say what the server does not do.
func TestTheProtocolDocumentNamesTheCallsTheServerMakes(t *testing.T) {
	text, err := os.ReadFile("../../docs/PROTOCOL.md")
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, found := range regexp.MustCompile("(?m)^\\| `((?:GET|POST|PUT|DELETE) /api/[^`]*)` \\|").FindAllStringSubmatch(string(text), -1) {
		documented[found[1]] = true
	}
	here := open(t)
	made := []string{"GET /api/protocol"}
	for _, c := range here.server.protocol() {
		made = append(made, c.Method+" "+c.Path)
	}
	for _, call := range made {
		if !documented[call] {
			t.Errorf("docs/PROTOCOL.md does not name %s", call)
		}
	}
	for call := range documented {
		if !slices.Contains(made, call) {
			t.Errorf("docs/PROTOCOL.md names %s, which the server does not have", call)
		}
	}
}

// A client can find out which protocol a server speaks, without signing in.
func TestAClientAsksTheServerWhichProtocolItSpeaks(t *testing.T) {
	here := open(t)
	got := here.ask(t, "GET", "/api/protocol", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	var said struct {
		Protocol int    `json:"protocol"`
		Calls    []call `json:"calls"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &said); err != nil {
		t.Fatal(err)
	}
	if said.Protocol != ProtocolVersion || len(said.Calls) != len(here.server.protocol()) {
		t.Errorf("the server said version %d and %d calls", said.Protocol, len(said.Calls))
	}
}
