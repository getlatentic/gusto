package web

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/helper/cli"
)

type oneToken struct{ token string }

func (o oneToken) Get(string) (string, error) { return o.token, nil }
func (o oneToken) Set(string, string) error   { return nil }
func (o oneToken) Delete(string) error        { return nil }

// A chat started for a folder on a person's computer makes an agent, the helper finds it by the folder's key, and then what is saved in
// the folder reaches the agent's folder and what the agent writes reaches the folder.
func TestAFolderOnThePersonsComputerIsKeptTheSameAsTheirAgentsFolder(t *testing.T) {
	here := withTokens(t)
	server := httptest.NewServer(here.handler)
	defer server.Close()
	token, _, err := helper.Login{
		Server: server.URL, Label: "Ada's laptop", Wait: func(context.Context, time.Duration) error { return nil },
		Show: func(_, code string) { here.as(t, alice).ask(t, "POST", "/device", url.Values{"code": {code}}) },
	}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	key := "0123456789abcdef"
	started := here.as(t, alice).ask(t, "POST", "/chat", url.Values{"harness": {"fixture"}, "said": {"hello"}, "folder": {key}, "folder_name": {"thesis"}})
	where := started.Header().Get("Location")
	if !strings.Contains(where, "/threads/") {
		t.Fatalf("a chat for a folder answered %q", where)
	}
	instance := strings.Split(strings.TrimPrefix(where, "/instances/"), "/")[0]
	workspace := here.server.instances.Workspace(instance)

	local := filepath.Join(t.TempDir(), "thesis")
	os.MkdirAll(local, 0o755)
	os.WriteFile(filepath.Join(local, "chapter.md"), []byte("by hand"), 0o644)
	folders := &cli.FolderSync{Server: server.URL, Vault: oneToken{token}, Dir: t.TempDir()}
	if err := folders.Open(); err != nil {
		t.Fatal(err)
	}
	if err := folders.Register(key, local); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { folders.Run(ctx); close(done) }()
	defer func() { stop(); <-done }()

	waitLong(t, "the person's file to reach the agent's folder", func() bool {
		text, err := os.ReadFile(filepath.Join(workspace, "chapter.md"))
		return err == nil && string(text) == "by hand"
	})
	os.WriteFile(filepath.Join(workspace, "from-the-agent.md"), []byte("by the agent"), 0o644)
	waitLong(t, "the agent's file to reach the person's folder", func() bool {
		text, err := os.ReadFile(filepath.Join(local, "from-the-agent.md"))
		return err == nil && string(text) == "by the agent"
	})
}

func waitLong(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("waited for %s, and it did not happen", what)
}
