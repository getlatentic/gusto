package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An agent is known by its own mark wherever it is listed, so each one the binary ships has its file.
func TestEveryShippedAgentHasItsOwnMark(t *testing.T) {
	adapters, err := filepath.Glob("../harness/adapters/*.json")
	if err != nil || len(adapters) == 0 {
		t.Fatalf("no adapters found: %v", err)
	}
	for _, path := range adapters {
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		if logo(name) == "" {
			t.Errorf("%s has no mark under static/agents", name)
		}
	}
}
