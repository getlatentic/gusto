package doctor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Outcome is what one command did. A command that never started, because the
// program is not there, has no exit code, and Started says so.
type Outcome struct {
	Started bool
	Code    int
	Output  string
}

// Commands runs a program and reports what it did. It is a value rather than a
// call to exec, so a test can be a runtime that answers, refuses or hands back
// an empty directory without a container runtime on the machine.
type Commands func(program string, args ...string) Outcome

// Machine is what the checks ask of the host. Settings come from the
// environment, the same place the runner reads them.
type Machine struct {
	Commands Commands
	LookPath func(program string) (string, error)
	Writable func(dir string) error
	User     func() int
}

// This is the machine the command runs on.
func This() Machine {
	return Machine{
		Commands: execute,
		LookPath: exec.LookPath,
		Writable: canWrite,
		User:     os.Getuid,
	}
}

// A container start that has not answered in two minutes is not going to, and a
// check that hangs is worse than one that fails.
const patience = 2 * time.Minute

func execute(program string, args ...string) Outcome {
	ctx, cancel := context.WithTimeout(context.Background(), patience)
	defer cancel()
	output, err := exec.CommandContext(ctx, program, args...).CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Outcome{Started: true, Output: string(output)}
	case errors.As(err, &exit):
		return Outcome{Started: true, Code: exit.ExitCode(), Output: string(output)}
	default:
		return Outcome{Output: err.Error()}
	}
}

// canWrite leaves a file and takes it away again, because a directory's mode
// bits do not say what this user may do under a read-only mount or a systemd
// sandbox.
func canWrite(dir string) error {
	file, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	file.Close()
	return os.Remove(file.Name())
}

// lastLine is the part of a command's output worth repeating: a runtime says
// why it refused on the line it ends with.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
