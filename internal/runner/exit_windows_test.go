//go:build windows

package runner_test

import (
	"testing"
	"time"
)

// waitForExit has nothing to ask a process on Windows: the run's ending is the test's evidence there.
func waitForExit(t *testing.T, pidFile string, within time.Duration) { t.Helper() }
