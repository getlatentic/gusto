package runner

import (
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

func TestAServerTheImageDoesNotCarryIsNamedAsMissing(t *testing.T) {
	_, err := located([]job.Server{{Name: "memory", Command: "no-such-mcp-server-anywhere"}})
	if err == nil {
		t.Fatal("a server with no program was accepted")
	}
	found, err := located([]job.Server{{Name: "shell", Command: "sh"}})
	if err != nil || found[0].Command == "sh" || found[0].Command == "" {
		t.Errorf("sh was not resolved to its full path: %+v, %v", found, err)
	}
}
