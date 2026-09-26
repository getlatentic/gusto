// Package runner does the work: the harness, then the gate, then the evidence.
//
// This runs inside the sandbox. A harness that drives an external CLI could be
// contained by wrapping that CLI, but a harness with its own tool loop runs its
// edits and shell in its own process — so the boundary goes around the runner,
// and then every harness is isolated the same way.
package runner

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/rules"
	"github.com/getlatentic/haas/internal/signing"
)

// Inside does the whole run and returns the exit code the gate earned, so a
// caller needs nothing but the process status to know what happened.
func Inside(dir job.Dir) (int, error) {
	stopRelay, err := relayOut()
	if err != nil {
		return 0, err
	}
	defer stopRelay()
	// A key that is named but unreadable stops the run before the agent does
	// any work, rather than leaving a record unsigned that was meant to be.
	key, keyed, err := signing.Configured()
	if err != nil {
		return 0, err
	}
	spec, err := job.Read(dir)
	if err != nil {
		return 0, err
	}
	adapter, err := harness.Load(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	endpoint := adapter.EndpointOf()
	if err := rules.Allows(spec, endpoint); err != nil {
		return 0, err
	}
	if adapter.Needs == harness.NeedsLocal {
		if err := Reachable(endpoint); err != nil {
			return 0, err
		}
	}
	if err := dir.Make(); err != nil {
		return 0, err
	}
	if err := Fill(spec, dir); err != nil {
		return 0, err
	}

	started := evidence.Now()
	record, err := Open(dir, adapter)
	if err != nil {
		return 0, err
	}
	budget := time.Duration(spec.Budget.Seconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	exit, err := stream(ctx, adapter.Command(ctx, spec, dir.Workspace()), record)
	tokens, cost := record.Spent()
	if closeErr := record.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}

	if err := Diff(dir); err != nil {
		return 0, err
	}
	// A turn in a conversation names no gate. It ran, and the evidence says what
	// happened without claiming anything passed.
	var verdict *evidence.Verdict
	if len(spec.Gate) > 0 {
		given, err := Gate(spec, dir)
		if err != nil {
			return 0, err
		}
		verdict = &given
	}
	capped, err := stoppedByCap(dir, started)
	if err != nil {
		return 0, err
	}
	spent, files, err := evidence.Of(dir)
	if err != nil {
		return 0, err
	}
	if err := evidence.Write(dir, evidence.Manifest{
		Job:             spent,
		Files:           files,
		Gate:            verdict,
		Harness:         adapter.Name,
		Session:         record.Remembered,
		HarnessExit:     exit,
		StoppedAtBudget: ctx.Err() == context.DeadlineExceeded,
		StoppedByCap:    capped,
		ModelEndpoint:   endpoint,
		Tokens:          tokens,
		CostUSD:         cost,
		Signer:          signer(key, keyed),
		AtUnix:          evidence.Now(),
	}); err != nil {
		return 0, err
	}
	if keyed {
		if err := evidence.Seal(dir, key); err != nil {
			return 0, err
		}
	}
	if verdict == nil || verdict.Passed {
		return 0, nil
	}
	return 1, nil
}

type said struct {
	text   string
	stderr bool
}

// stream reads both pipes into one order and keeps every line. The scanner is
// given room because an agent's JSON line is routinely longer than the 64 KiB
// a bufio.Scanner takes by default, and a dropped line is lost evidence.
func stream(ctx context.Context, command *exec.Cmd, record *Record) (int, error) {
	out, err := command.StdoutPipe()
	if err != nil {
		return 0, err
	}
	errs, err := command.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := command.Start(); err != nil {
		return 0, err
	}
	lines := make(chan said, 256)
	var readers sync.WaitGroup
	readers.Add(2)
	go scan(out, false, lines, &readers)
	go scan(errs, true, lines, &readers)
	go func() {
		readers.Wait()
		close(lines)
	}()
	for line := range lines {
		var err error
		if line.stderr {
			err = record.Err(line.text)
		} else {
			err = record.Out(line.text)
		}
		if err != nil {
			return 0, err
		}
	}
	waitErr := command.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return command.ProcessState.ExitCode(), nil
	}
	var exit *exec.ExitError
	if waitErr != nil && !asExit(waitErr, &exit) {
		return 0, waitErr
	}
	return command.ProcessState.ExitCode(), nil
}

func scan(pipe io.Reader, stderr bool, lines chan<- said, done *sync.WaitGroup) {
	defer done.Done()
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines <- said{text: scanner.Text(), stderr: stderr}
	}
}

func asExit(err error, into **exec.ExitError) bool {
	exit, ok := err.(*exec.ExitError)
	if ok {
		*into = exit
	}
	return ok
}

// signer is the name the manifest gives for who stands behind it. It is the
// runner's own word: a verifier names the signer from its keyring instead. A
// runner with a key and no name gives the key's id, so a signed record never
// calls itself unsigned.
func signer(key signing.Key, keyed bool) string {
	if name := os.Getenv("HAAS_SIGNER"); name != "" {
		return name
	}
	if keyed {
		return key.ID()
	}
	return "unsigned"
}
