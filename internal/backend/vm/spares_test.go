package vm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/openshell/openshelltest"
)

// A spare made with the model URL of a proxy port that was replaced is removed when the server starts with the new port, and the
// spare made in its place names the new port (getlatentic/planning#843).
func TestASpareMadeWithAReplacedProxyPortIsRemovedAndRemadeWithTheNewOne(t *testing.T) {
	root := openshelltest.Install(t)
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	gateway := openshell.Config{Binary: "openshell", Image: "runner.tar", CPU: "1", Memory: "1Gi", Host: "host.openshell.internal"}
	if err := openshell.MakeBlank(context.Background(), gateway, computer.SpareName("acme-com"), "http://host.openshell.internal:7001/model/v1"); err != nil {
		t.Fatal(err)
	}
	old := sparesIn(t, root)
	if len(old) != 1 {
		t.Fatalf("the gateway holds %v, want the one old spare", old)
	}

	b := New(Config{Gateway: gateway, Computers: registry, Org: "acme-com", Node: "local", Spare: true, ProxyPort: "7002"})
	stop, err := b.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	var now []string
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if now = sparesIn(t, root); len(now) == 1 && now[0] != old[0] {
			break
		}
	}
	if len(now) != 1 || now[0] == old[0] {
		t.Fatalf("the gateway holds %v, want one new spare in place of %v", now, old)
	}
	policy, err := os.ReadFile(filepath.Join(root, now[0], "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(policy), "7002") || strings.Contains(string(policy), "7001") {
		t.Errorf("the new spare's policy does not name only the new proxy port:\n%s", policy)
	}
}

func sparesIn(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), computer.SparePrefix("acme-com")) {
			found = append(found, entry.Name())
		}
	}
	return found
}
