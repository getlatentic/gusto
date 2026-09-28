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
	agent := agentHere()
	agent.trustWorkspace()
	// The digest is taken here, from what the job carries, before the harness
	// starts: nothing it writes during the run can change what the manifest
	// says it was.
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	adapter := configured.Adapter
	if err := admit(spec, adapter); err != nil {
		return 0, Refuse(dir, err)
	}
	endpoint := adapter.EndpointOf()
	if err := dir.Make(); err != nil {
		return 0, err
	}
	if err := Fill(spec, dir); err != nil {
		return 0, err
	}
	before, err := Snapshot(dir)
	if err != nil {
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
	servers, err := Servers(dir)
	if err != nil {
		return 0, Refuse(dir, err)
	}
	if adapter.Keeps != "" {
		if err := os.MkdirAll(adapter.Keeps, 0o700); err != nil {
			return 0, err
		}
	}
	if err := placeSkills(dir, spec, adapter); err != nil {
		return 0, err
	}
	command := adapter.Command(ctx, spec, dir.Workspace())
	var exit int
	if adapter.Stream != harness.ACP {
		err = adapter.Give(command, servers)
	}
	if err == nil {
		err = agent.handOver(dir, command)
	}
	if err == nil && adapter.Stream == harness.ACP {
		exit, err = converse(ctx, command, spec, servers, record)
	} else if err == nil {
		exit, err = stream(ctx, command, record)
	}
	agent.endAll()
	tokens, cost := record.Spent()
	if closeErr := record.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}

	if err := Diff(dir, before); err != nil {
		return 0, err
	}
	// A turn in a conversation names no gate. It ran, and the evidence says what
	// happened without claiming anything passed.
	var verdict *evidence.Verdict
	if len(spec.Gate) > 0 {
		given, err := Gate(spec, dir)
		agent.endAll()
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
		HarnessDigest:   configured.Digest,
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
	return earned(verdict, exit), nil
}

// earned is the exit code a run earned. A gate decides when there is one. A
// turn in a conversation has none, and then the harness's own exit is the only
// word on whether it did what it was asked: a harness that failed made a turn
// that failed, whatever it managed to say first.
func earned(verdict *evidence.Verdict, harnessExit int) int {
	switch {
	case verdict != nil && verdict.Passed:
		return 0
	case verdict != nil:
		return 1
	case harnessExit != 0:
		return 1
	}
	return 0
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
