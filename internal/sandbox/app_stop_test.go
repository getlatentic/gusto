//go:build !windows

package sandbox

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

// Starting an app removes a container an earlier start of the same app left, then starts its own and leaves it running.
func TestStartingAnAppReplacesTheContainerAnEarlierStartLeft(t *testing.T) {
	script, log := runtimeThat(t, "  echo started")
	c := with(script, "gusto:dev")
	err := c.StartApp(context.Background(), AppBox{Name: "gusto-app-acme-a", App: "a", Folder: t.TempDir(), SocketDir: t.TempDir(), Command: "c", Port: 3000})
	if err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	removed := slices.Index(lines, "rm -f gusto-app-acme-a")
	run := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "run -d --name gusto-app-acme-a ") {
			run = i
		}
	}
	if removed < 0 || run < removed {
		t.Errorf("the old container is removed before the new one starts: %q", lines)
	}
}
