package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// containers is the names of the turns' containers that are running.
func (h host) containers() ([]string, error) {
	out, err := exec.Command("docker", "ps", "--filter", "name=^gusto-", "--format", "{{.Names}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, out)
	}
	return strings.Fields(string(out)), nil
}

type probeResult struct {
	output string
	err    error
}

// probeFirstContainer waits for the first turn's container and runs the isolation check inside it while the agent works, since
// a container lives only as long as its turn. It is nil when the agents do not run in containers.
func (h host) probeFirstContainer(within time.Duration) <-chan probeResult {
	if h.sandbox != "container" {
		return nil
	}
	result := make(chan probeResult, 1)
	go func() {
		deadline := time.Now().Add(within)
		for time.Now().Before(deadline) {
			running, err := h.containers()
			if err != nil {
				result <- probeResult{err: err}
				return
			}
			if len(running) > 0 {
				out, err := exec.Command("bash", h.probe).CombinedOutput()
				result <- probeResult{output: string(out), err: err}
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		result <- probeResult{err: fmt.Errorf("no turn's container ran within %s", within)}
	}()
	return result
}
