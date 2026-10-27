package openshell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/landlock"
	"github.com/getlatentic/gusto/internal/runtoken"
)

// startInComputer has the workspace there, and puts this task's workspace in a directory of
// the task's own. The gateway is asked whether the sandbox is there, and what is made
// is made once: a person's files and their agents' state are what a workspace keeps.
func (s *Sandbox) startInComputer(ctx context.Context, workspace string) error {
	if _, err := output(s.run(ctx, "sandbox", "get", s.name)); err != nil {
		if err := s.makeSandboxOnly(ctx); err != nil {
			return err
		}
	} else if s.answers(ctx) != nil {
		// It is there and does not answer, which is a workspace that was stopped.
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
		return fmt.Errorf("the directory is %s, and a workspace's directory is named workspace", absolute)
	}
	here := jobsDir + "/" + s.job
	// What a turn of a conversation left there is the last turn's, and the workspace it is
	// given now is the whole of what it works on.
	steps := [][]string{{"rm", "-rf", here}, {"mkdir", "-p", here}}
	if s.confined {
		// The home and the temporary folder outlive the turn, and Landlock allows only a folder that exists.
		steps = append(steps, []string{"mkdir", "-p", s.home, s.temp})
	}
	for _, command := range steps {
		if _, err := output(s.run(ctx, append([]string{"sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--"}, command...)...)); err != nil {
			return err
		}
	}
	if _, err := output(s.run(ctx, "sandbox", "upload", s.name, absolute, here)); err != nil {
		return err
	}
	// The log is the workspace's, and what this task did is what is added to it now.
	logged, err := output(s.run(ctx, "logs", "-n", most, s.name))
	if err != nil {
		return err
	}
	s.logBase = bytes.Count(logged, []byte("\n"))
	return nil
}

// makeSandboxOnly makes the sandbox with its policy and nothing in it.
func (s *Sandbox) makeSandboxOnly(ctx context.Context) error {
	policy, err := os.CreateTemp("", "gusto-sandbox-policy-")
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

// answering waits until the sandbox runs a command, which a workspace that was
// stopped and started does a moment after it is asked to.
func (s *Sandbox) answering(ctx context.Context) error {
	deadline := time.Now().Add(readyWithin)
	for {
		if s.answers(ctx) == nil {
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fmt.Errorf("the workspace %s did not answer in %s", s.name, readyWithin)
		}
		time.Sleep(retryEvery)
	}
}

// readyWithin is how long a workspace has to run a command once it is started.
var readyWithin = 2 * time.Minute

// jobEvents is the part of a workspace's log that is this task's: what came after the
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

// endJob takes away what this task left in a workspace, which is the directory it
// worked in and the skills it was given: the next task is given its own. The workspace
// stays, and so does the directory of a conversation, which its next turn works in.
func (s *Sandbox) endJob() error {
	if s.keepDir {
		return nil
	}
	if _, err := output(s.run(context.Background(), "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
		"rm", "-rf", jobsDir+"/"+s.job)); err != nil {
		return err
	}
	return nil
}

// Delete removes a sandbox by its name, and a sandbox that is not there is removed.
func Delete(cfg Config, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), removeWithin)
	defer cancel()
	return Remove(ctx, cfg, name)
}

// Stop stops a sandbox and keeps what is in it.
func Stop(cfg Config, name string) error {
	_, err := output((&Sandbox{cfg: cfg}).run(context.Background(), "sandbox", "stop", name))
	return err
}

// Wake has a workspace answering, starting it if it was stopped, which is what a person's
// workspace is after nobody used it for a while.
func Wake(ctx context.Context, cfg Config, name string) error {
	s := &Sandbox{cfg: cfg, name: name}
	if s.answers(ctx) != nil {
		if _, err := output(s.run(ctx, "sandbox", "start", name)); err != nil {
			return err
		}
	}
	return s.answering(ctx)
}

// gustoInImage is gusto in the runner image, which starts an agent with its forwarder and its Landlock policy.
const gustoInImage = "/usr/local/bin/gusto"

// Confine holds every command of the task to folders of its own in the person's computer: the workspace it is given, and a
// home and a temporary folder kept for key under /sandbox/rooms, so what the agent remembers carries to its next turn. The rest
// of the computer, the person's other conversations and rooms among it, cannot be read or written. The rules are the kernel's,
// Landlock, applied by gusto in the sandbox on top of the sandbox's own; what the agent is told plays no part.
func (s *Sandbox) Confine(key string) {
	kept := Home + "/rooms/" + dirName(key)
	s.home, s.temp, s.confined = kept+"/home", kept+"/tmp", true
}

// held is the command as the sandbox runs it: through gusto boxed, which starts the run's forwarder, when the task is bound;
// behind the task's Landlock policy, with its own home and temporary folder, when it is confined; and as it is otherwise.
func (s *Sandbox) held(command *exec.Cmd) (*exec.Cmd, error) {
	// Only the agent is told the model's address. The gate and the checks of the image are not, and do not spend the run's secret.
	forwarded := s.bound && mentions(command, runtoken.Place)
	if !s.confined && !forwarded {
		return command, nil
	}
	held := *command
	var front []string
	if forwarded {
		front = []string{s.program(), "boxed", "--model", s.proxyRoot()}
	} else {
		front = []string{s.program(), "sandbox-run"}
	}
	if s.confined {
		policy, err := landlock.Policy{Write: []string{jobsDir + "/" + s.job, s.home}, Temp: s.temp}.JSON()
		if err != nil {
			return nil, err
		}
		if forwarded {
			front = append(front, "--policy")
		}
		front = append(front, policy)
		held.Env = append(slices.Clone(command.Env), "HOME="+s.home, "TMPDIR="+s.temp)
	}
	held.Args = append(append(front, "--"), command.Args...)
	return &held, nil
}

// mentions says whether a command's arguments or environment name text.
func mentions(command *exec.Cmd, text string) bool {
	return slices.ContainsFunc(command.Args, func(each string) bool { return strings.Contains(each, text) }) ||
		slices.ContainsFunc(command.Env, func(each string) bool { return strings.Contains(each, text) })
}

// program is the gusto that starts an agent in the sandbox: the image's, unless the configuration names another, which a test does.
func (s *Sandbox) program() string {
	if s.cfg.Gusto != "" {
		return s.cfg.Gusto
	}
	return gustoInImage
}
