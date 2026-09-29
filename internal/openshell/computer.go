package openshell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// startInComputer has the computer there, and puts this job's workspace in a directory of
// the job's own. The gateway is asked whether the sandbox is there, and what is made
// is made once: a person's files and their harnesses' state are what a computer keeps.
func (s *Sandbox) startInComputer(ctx context.Context, workspace string) error {
	if _, err := output(s.run(ctx, "sandbox", "get", s.name)); err != nil {
		if err := s.makeSandboxOnly(ctx); err != nil {
			return err
		}
	} else if s.answers(ctx) != nil {
		// It is there and does not answer, which is a computer that was stopped.
		if _, err := output(s.run(ctx, "sandbox", "start", s.name)); err != nil {
			return err
		}
	}
	if err := s.answering(ctx); err != nil {
		return err
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	if filepath.Base(absolute) != "workspace" {
		return fmt.Errorf("the workspace is %s, and a computer names it workspace", absolute)
	}
	here := jobsDir + "/" + s.job
	if _, err := output(s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--", "mkdir", "-p", here)); err != nil {
		return err
	}
	if _, err := output(s.run(ctx, "sandbox", "upload", s.name, absolute, here)); err != nil {
		return err
	}
	// The log is the computer's, and what this job did is what is added to it now.
	logged, err := output(s.run(ctx, "logs", "-n", most, s.name))
	if err != nil {
		return err
	}
	s.logBase = bytes.Count(logged, []byte("\n"))
	return nil
}

// makeSandboxOnly makes the sandbox with its policy and nothing in it.
func (s *Sandbox) makeSandboxOnly(ctx context.Context) error {
	policy, err := os.CreateTemp("", "haas-sandbox-policy-")
	if err != nil {
		return err
	}
	defer os.Remove(policy.Name())
	if _, err := policy.Write(s.policy); err != nil {
		policy.Close()
		return err
	}
	policy.Close()
	args := []string{"sandbox", "create", "--name", s.name, "--policy", policy.Name(),
		"--cpu", s.cfg.CPU, "--memory", s.cfg.Memory, "--no-tty", "--detach", "--no-auto-providers"}
	if s.cfg.Image != "" {
		args = append(args, "--from", s.cfg.Image)
	}
	return s.create(ctx, append(args, "--", "sleep", "infinity"))
}

// answers is whether the sandbox runs a command now.
func (s *Sandbox) answers(ctx context.Context) error {
	_, err := output(s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--", "true"))
	return err
}

// answering waits until the sandbox runs a command, which a computer that was
// stopped and started does a moment after it is asked to.
func (s *Sandbox) answering(ctx context.Context) error {
	deadline := time.Now().Add(readyWithin)
	for {
		if s.answers(ctx) == nil {
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("the computer %s did not answer in %s", s.name, readyWithin)
		}
		time.Sleep(retryEvery)
	}
}

// readyWithin is how long a computer has to run a command once it is started.
var readyWithin = 2 * time.Minute

// jobEvents is the part of a computer's log that is this job's: what came after the
// lines that were there when it started. A log that is shorter than that, because
// it was cut, is kept whole rather than dropped.
func (s *Sandbox) jobEvents(all []byte) []byte {
	if !s.persistent || s.logBase == 0 {
		return all
	}
	lines := bytes.SplitAfter(all, []byte("\n"))
	if len(lines) <= s.logBase {
		return all
	}
	return bytes.Join(lines[s.logBase:], nil)
}

// endJob takes away what this job left in a computer, which is the directory it
// worked in and the skills it was given: the next job is given its own. The computer
// stays.
func (s *Sandbox) endJob() error {
	if _, err := output(s.run(context.Background(), "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
		"rm", "-rf", jobsDir+"/"+s.job)); err != nil {
		return err
	}
	return nil
}

// Delete removes a sandbox by its name, and a sandbox that is not there is removed.
func Delete(cfg Config, name string) error {
	if _, err := output((&Sandbox{cfg: cfg}).run(context.Background(), "sandbox", "delete", name)); err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

// Stop stops a sandbox and keeps what is in it.
func Stop(cfg Config, name string) error {
	_, err := output((&Sandbox{cfg: cfg}).run(context.Background(), "sandbox", "stop", name))
	return err
}
