//go:build githublive

package repoaccess_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/repoaccess"
)

// This runs against a real GitHub App, which the ordinary tests stand in for.
// Install the App on a repository, give its id and key in the settings, then:
//
//	GUSTO_TEST_GITHUB_REPO=acme/api GUSTO_TEST_GITHUB_OTHER=acme/not-installed \
//	  GUSTO_GITHUB_APP_ID=... GUSTO_GITHUB_APP_KEY=app.pem \
//	  go test -tags githublive -run Live ./internal/repoaccess
//
// GUSTO_TEST_GITHUB_OTHER is optional: a repository the App is not installed on.
func TestLiveAClonePutsTheRepositoryDownAndTheTokenIsGoneAfter(t *testing.T) {
	repo := os.Getenv("GUSTO_TEST_GITHUB_REPO")
	if repo == "" {
		t.Skip("GUSTO_TEST_GITHUB_REPO names no repository")
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

	// A repository the App is not installed on is refused by name.
	if other := os.Getenv("GUSTO_TEST_GITHUB_OTHER"); other != "" {
		var refused repoaccess.NotInstalledError
		err := forge.Check(ctx, forge.Web+"/"+other+".git")
		if !errors.As(err, &refused) || refused.Repo != other {
			t.Errorf("%s: got %v, want a refusal that names it", other, err)
		}
	}

	// A token minted the same way, then revoked, is refused by GitHub. GitHub
	// refuses it over the API at once and over git a few seconds later, so this
	// waits for both and says how long each took.
	located, _ := forge.Locate(address)
	token, err := forge.App.Token(ctx, located.Owner, located.Name, repoaccess.Read)
	if err != nil {
		t.Fatal(err)
	}
	if err := token.Revoke(ctx); err != nil {
		t.Fatal(err)
	}
	refused := func(url, auth string) int {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		request.Header.Set("Authorization", auth)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return 0
		}
		response.Body.Close()
		return response.StatusCode
	}
	api := forge.API + "/installation/repositories"
	git := forge.Web + "/" + located.String() + ".git/info/refs?service=git-upload-pack"
	basic := "basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token.Value))
	start := time.Now()
	var apiAt, gitAt time.Duration
	for time.Since(start) < 15*time.Second && (apiAt == 0 || gitAt == 0) {
		if apiAt == 0 && refused(api, "Bearer "+token.Value) == http.StatusUnauthorized {
			apiAt = time.Since(start)
		}
		if gitAt == 0 && refused(git, basic) == http.StatusUnauthorized {
			gitAt = time.Since(start)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if apiAt == 0 || gitAt == 0 {
		t.Fatalf("a revoked token was still honored after 15s: api %v, git %v", apiAt, gitAt)
	}
	t.Logf("a revoked token was refused over the API after %v and over git after %v", apiAt.Round(100*time.Millisecond), gitAt.Round(100*time.Millisecond))
}
