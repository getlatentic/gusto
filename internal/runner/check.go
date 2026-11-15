package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/pkg/task"
)

// Ask runs the command that says pass or fail and writes nothing, until it ends or ctx does. An agent reporting success is a
// claim; this is the check, and a re-run of it has to leave the record it is checking untouched.
func Ask(ctx context.Context, spec task.Spec, dir task.Dir) (record.Verdict, []byte, error) {
	command, err := checkCommand(ctx, spec, dir, nil, agent{})
	if err != nil {
		return record.Verdict{}, nil, err
	}
	return askWith(spec, command)
}

// checkCommand is the task's check, to run in its working folder as the agent with env, or this process's environment when env is nil.
func checkCommand(ctx context.Context, spec task.Spec, dir task.Dir, env []string, as agent) (*exec.Cmd, error) {
	if len(spec.Check) == 0 {
		return nil, errors.New("the task names no check")
	}
	command := exec.CommandContext(ctx, spec.Check[0], spec.Check[1:]...)
	command.Dir = dir.WorkingFolder()
	command.Env = env
	// The check runs the repository's own code, as untrusted as the harness.
	as.runAs(command)
	return command, nil
}

// askWith runs a check command already prepared, in a box or here.
func askWith(spec task.Spec, command *exec.Cmd) (record.Verdict, []byte, error) {
	output, err := command.CombinedOutput()
	return record.Verdict{
		Command: spec.Check,
		Code:    command.ProcessState.ExitCode(),
		Passed:  err == nil,
	}, output, nil
}

// checkWith runs the check within the run's ctx, in a process group that ends with it, behind a wall when wrap is given, and
// keeps the answer and the whole output in the record.
func checkWith(ctx context.Context, spec task.Spec, dir task.Dir, env []string, wrap func(*exec.Cmd), as agent) (record.Verdict, error) {
	command, err := checkCommand(ctx, spec, dir, env, as)
	if err != nil {
		return record.Verdict{}, err
	}
	ownGroup(command)
	if wrap != nil {
		wrap(command)
	}
	verdict, output, err := askWith(spec, command)
	if err != nil {
		return record.Verdict{}, err
	}
	return verdict, keepCheck(dir, verdict, output)
}

// keepCheck writes the check's answer and its whole output into the record.
func keepCheck(dir task.Dir, verdict record.Verdict, output []byte) error {
	if err := os.WriteFile(dir.OutFile("gate.log"), output, 0o644); err != nil {
		return err
	}
	text, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile("gates.json"), append(text, '\n'), 0o644)
}
