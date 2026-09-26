package ship_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/ship"
)

func TestGitHubsRefusalIsSaidInFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("the token should be sent as a bearer, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed", "errors": [{"message": "A pull request already exists for o:haas/x."}]}`))
	}))
	defer server.Close()
	_, err := ship.GitHub{API: server.URL, Token: "secret"}.Open(context.Background(), ship.Pull{Owner: "o", Name: "r"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want GitHub's own reason, got %v", err)
	}
}

func TestATokenIsNeededToOpenAnything(t *testing.T) {
	t.Setenv("HAAS_GITHUB_TOKEN", "")
	if _, err := ship.GitHubFromEnv(); err == nil {
		t.Fatal("no token should be an error before anything is pushed")
	}
	t.Setenv("HAAS_GITHUB_TOKEN", "t")
	t.Setenv("HAAS_GITHUB_API", "http://example.test/")
	forge, err := ship.GitHubFromEnv()
	if err != nil || forge.API != "http://example.test" {
		t.Fatalf("api %q, err %v", forge.API, err)
	}
}
