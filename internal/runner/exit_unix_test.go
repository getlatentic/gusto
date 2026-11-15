//go:build !windows

package runner_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// waitForExit fails the test when the process whose id is in the file is still running after the time given.
func waitForExit(t *testing.T, pidFile string, within time.Duration) {
	t.Helper()
	text, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		t.Fatalf("%s holds %q", pidFile, text)
	}
	deadline := time.Now().Add(within)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("process %d from %s was still running %v after the stop", pid, filepath.Base(pidFile), within)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
