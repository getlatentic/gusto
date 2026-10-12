package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A key in the person's shell is added to their own haas by the command they run, through the chat page's form: it signs in
// with a link of its own, and the provider and the model are on.
func TestAKeyInThePersonsShellIsAddedToTheirOwnHaas(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "small"}}})
	}))
	t.Cleanup(provider.Close)
	home := filepath.Join(t.TempDir(), "haas")
	setup, err := ensureLocal(home)
	if err != nil {
		t.Fatal(err)
	}
	setup.Port, setup.ProxyPort = freePort(0), freePort(0)
	if err := setup.save(home); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAAS_KEYCHAIN", "off")
	// runLocal sets these for the server it starts; they are put back when the test ends.
	for _, name := range []string{"HAAS_PASSWORD", "HAAS_ADDR", "HAAS_PROXY_ADDR", "HAAS_LOCAL", "HAAS_REGISTRY", "HAAS_REGISTRY_PRIVATE", "HAAS_SANDBOX", "PATH"} {
		t.Setenv(name, os.Getenv(name))
	}
	t.Chdir(t.TempDir())
	go runLocal(home)
	if !answers(fmt.Sprintf("http://127.0.0.1:%d/healthz", setup.Port), 20*time.Second) {
		t.Fatal("haas did not start")
	}
	keyProviders["HAAS_TEST_KEY"] = keyProvider{Address: provider.URL}
	t.Cleanup(func() { delete(keyProviders, "HAAS_TEST_KEY") })
	t.Setenv("HAAS_TEST_KEY", "sk-from-the-shell")

	if code := useKey(home, []string{"HAAS_TEST_KEY", "small"}); code != 0 {
		t.Fatalf("haas local use ended %d", code)
	}
	text, err := os.ReadFile(filepath.Join(home, "registry", "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var held struct {
		Providers []struct{ Name, Key string }
		Models    []struct{ Provider, Name string }
	}
	if err := json.Unmarshal(text, &held); err != nil {
		t.Fatal(err)
	}
	if len(held.Providers) != 1 || held.Providers[0].Key != "stored" || !slices.ContainsFunc(held.Models, func(m struct{ Provider, Name string }) bool { return m.Name == "small" }) {
		t.Errorf("the key's provider and model are not in the registry: %s", text)
	}
	if strings.Contains(string(text), "sk-from-the-shell") {
		t.Error("the key is in the registry's file")
	}
	if code := useKey(home, []string{"NOT_A_KNOWN_KEY"}); code == 0 {
		t.Error("a key haas does not know the provider of was added")
	}
}

func TestOnlyTheNamesOfTheKeysInTheShellAreKept(t *testing.T) {
	for name := range keyProviders {
		t.Setenv(name, "")
	}
	t.Setenv("DEEPSEEK_API_KEY", "sk-not-to-be-kept")
	if found := foundKeys(); !slices.Equal(found, []string{"DEEPSEEK_API_KEY"}) {
		t.Errorf("found %v", found)
	}
}
