package runner

import (
	"testing"

	"github.com/getlatentic/gusto/pkg/task"
)

func TestAServerTheImageDoesNotCarryIsNamedAsMissing(t *testing.T) {
	_, err := located([]task.Server{{Name: "memory", Command: "no-such-mcp-server-anywhere"}})
	if err == nil {
		t.Fatal("a server with no program was accepted")
	}
	found, err := located([]task.Server{{Name: "shell", Command: "sh"}})
	if err != nil || found[0].Command == "sh" || found[0].Command == "" {
		t.Errorf("sh was not resolved to its full path: %+v, %v", found, err)
	}
}
