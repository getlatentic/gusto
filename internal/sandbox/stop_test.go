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

	"github.com/getlatentic/haas/internal/job"
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

func theJob(t *testing.T) job.Dir {
	t.Helper()
	dir := job.Dir(filepath.Join(t.TempDir(), "20261008-0900-a-turn"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAContainerIsToldToStopByAnInterruptAndRemovedByNameIfItStays(t *testing.T) {
	script, log := runtimeThat(t, "    trap 'echo heard-interrupt >> "+"$LOGFILE"+"; exit 130' INT\n    while true; do sleep 0.1; done")
	t.Setenv("HAAS_CONTAINER", script)
	t.Setenv("LOGFILE", log)
	dir := theJob(t)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := start(ctx, dir, []string{"run", "--rm", "--name", containerName(dir), "image"})
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
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
	if !strings.Contains(string(calls), "rm -f haas-20261008-0900-a-turn") {
		t.Errorf("a container that may still be there was not removed by name:\n%s", calls)
	}
}

func TestEveryJobsContainerHasAnAddressableName(t *testing.T) {
	args, err := argv("/var/lib/haas/jobs/20261008-0900-a-turn", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--name haas-20261008-0900-a-turn") {
		t.Errorf("the container has no name to be stopped by: %s", joined)
	}
}
