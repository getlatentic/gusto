//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// runtimeThat is a container runtime that is a script: a `run` takes an interrupt as the real client hands one to its
// container, writing down that it heard it, and sleeping otherwise; every other command is written down.
func runtimeThat(t *testing.T, onRun string) (script, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")
	script = filepath.Join(dir, "runtime")
	body := "#!/bin/sh\necho \"$@\" >> " + log + "\n" +
		"case \"$1\" in\n  run)\n" + onRun + "\n  ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, log
}

// waitForLine waits until the script has installed its trap, which it says by writing the line, so that the interrupt is
// heard and not lost to a shell that has not started yet.
func waitForLine(t *testing.T, path, line string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if text, _ := os.ReadFile(path); strings.Contains(string(text), line) {
			return
		}
		if time.Now().After(deadline) {
			t.Error("the script never started")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func theJob(t *testing.T) job.Dir {
	t.Helper()
	dir := job.Dir(filepath.Join(t.TempDir(), "20261008-0900-a-turn"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAContainerIsToldToStopByAnInterruptAndRemovedByNameIfItStays(t *testing.T) {
	script, log := runtimeThat(t, "    trap 'echo heard-interrupt >> "+"$LOGFILE"+"; exit 130' INT\n    echo ready >> "+"$LOGFILE"+"\n    while true; do sleep 0.1; done")
	t.Setenv("GUSTO_CONTAINER", script)
	t.Setenv("LOGFILE", log)
	dir := theJob(t)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := start(ctx, dir, []string{"run", "--rm", "--name", containerName(dir), "image"})
		done <- err
	}()
	waitForLine(t, log, "ready")
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("a stopped container ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the runtime was not stopped")
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "heard-interrupt") {
		t.Errorf("the container was not given an interrupt:\n%s", calls)
	}
	if !strings.Contains(string(calls), "rm -f gusto-20261008-0900-a-turn") {
		t.Errorf("a container that may still be there was not removed by name:\n%s", calls)
	}
}

func TestEveryJobsContainerHasAnAddressableName(t *testing.T) {
	args, err := argv("/var/lib/gusto/jobs/20261008-0900-a-turn", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--name gusto-20261008-0900-a-turn") {
		t.Errorf("the container has no name to be stopped by: %s", joined)
	}
}
