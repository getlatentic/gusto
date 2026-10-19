package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// startedTheOldWay writes an instance the way the two-step start left one on
// disk: a computer with a workspace, and no conversation opened on it.
func startedTheOldWay(t *testing.T, root, id string) {
	t.Helper()
	dir := filepath.Join(root, id)
	for _, each := range []string{"workspace", "state", "threads"} {
		if err := os.MkdirAll(filepath.Join(dir, each), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	text, err := json.Marshal(instance.Instance{ID: id, Harness: "fixture", Started: 1790000000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "instance.json"), text, 0o644); err != nil {
		t.Fatal(err)
	}
}

// openOver is a server over instances that were already on disk before it
// started, which is the only way an instance from the old start reaches it.
func openOver(t *testing.T, instances string) *site {
	t.Helper()
	index, err := jobs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := instance.Open(instances)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, store, nil, password, &Identity{}, offered(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &site{handler: server.Handler(), index: index}
}

// Every card under Running leads into a conversation, including one for an
// instance that was started before starting opened one.
func TestARunningCardWithNoThreadYetStillLeadsIntoAConversation(t *testing.T) {
	root := t.TempDir()
	startedTheOldWay(t, root, "20260926-1100-bare")
	site := openOver(t, root)
	site.signIn(t)

	body := site.ask(t, "GET", "/", nil).Body.String()
	into := `href="/instances/20260926-1100-bare/threads/1"`
	if !strings.Contains(body, into) {
		t.Fatalf("its card does not lead into a conversation:\n%s", body)
	}
	if strings.Contains(body, `href="/instances/20260926-1100-bare"`) {
		t.Error("its card still leads to the instance's page")
	}
	if strings.Contains(body, "0 threads") {
		t.Error("its card says it has no threads")
	}
	// And the conversation it leads into is there to be read.
	if answer := site.ask(t, "GET", "/instances/20260926-1100-bare/threads/1", nil); answer.Code != 200 {
		t.Errorf("the conversation it leads into gave %d", answer.Code)
	}
}
