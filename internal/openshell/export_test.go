package openshell

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The fake runs a sandbox's commands on this machine, where its user already has many processes, so a test sets the limit this
// machine already has: the command that sets it still runs, and changes nothing.
func init() {
	out, err := exec.Command("sh", "-c", "ulimit -u").Output()
	if n, convErr := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && convErr == nil {
		unsetProcesses = n
	}
}

// SetSettle lets a test wait milliseconds where a run waits seconds.
func SetSettle(wait time.Duration, rounds int) { settle, settleRounds = wait, rounds }

// PlaceConfig is placeConfig, for the tests.
func PlaceConfig(from, home string) string { return placeConfig(from, home) }

// SetRetry lets a test wait milliseconds where a run waits seconds.
func SetRetry(total, every time.Duration) { retryFor, retryEvery = total, every }
