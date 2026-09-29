//go:build githublive

package repoaccess_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/haas/internal/repoaccess"
)

// This runs against a real GitHub App, which the ordinary tests stand in for.
// Install the App on a repository, give its id and key in the settings, then:
//
//	HAAS_TEST_GITHUB_REPO=acme/api HAAS_GITHUB_APP_ID=... HAAS_GITHUB_APP_KEY=app.pem \
//	  go test -tags githublive -run Live ./internal/repoaccess
func TestLiveAClonePutsTheRepositoryDownAndTheTokenIsGoneAfter(t *testing.T) {
	repo := os.Getenv("HAAS_TEST_GITHUB_REPO")
	if repo == "" {
		t.Skip("HAAS_TEST_GITHUB_REPO names no repository")
	}
	forge, err := repoaccess.FromEnv()
	if err != nil || forge.App == nil {
		t.Fatalf("the settings name no App: %v", err)
	}
	ctx := context.Background()
	address := forge.Web + "/" + repo + ".git"
	if err := forge.Check(ctx, address); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := forge.Clone(ctx, address, dir, "workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workspace", ".git")); err != nil {
		t.Fatal(err)
	}

	// A token minted the same way, then revoked, is refused by GitHub.
	located, _ := forge.Locate(address)
	token, err := forge.App.Token(ctx, located.Owner, located.Name, repoaccess.Read)
	if err != nil {
		t.Fatal(err)
	}
	if err := token.Revoke(ctx); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, forge.API+"/installation/repositories", nil)
	request.Header.Set("Authorization", "Bearer "+token.Value)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("a revoked token answered %d, want 401", response.StatusCode)
	}
}
