package runner

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
)

// Ask runs the command that says pass or fail and writes nothing. A harness
// reporting success is a claim; this is the check, and a re-run of it has to
// leave the evidence it is checking untouched.
func Ask(spec job.Spec, dir job.Dir) (evidence.Verdict, []byte, error) {
	return askIn(spec, dir, nil, nil)
}

// askIn is Ask with the environment the gate runs with, or this process's when env is nil, and behind a wall when wrap is given.
func askIn(spec job.Spec, dir job.Dir, env []string, wrap func(*exec.Cmd)) (evidence.Verdict, []byte, error) {
	if len(spec.Gate) == 0 {
		return evidence.Verdict{}, nil, errors.New("the job names no gate")
	}
	command := exec.Command(spec.Gate[0], spec.Gate[1:]...)
	command.Dir = dir.Workspace()
	command.Env = env
	// The gate runs the repository's own code, as untrusted as the harness.
	agentHere().runAs(command)
	if wrap != nil {
		wrap(command)
	}
	return askWith(spec, command)
}

// askWith runs a gate command already prepared, in a box or here.
func askWith(spec job.Spec, command *exec.Cmd) (evidence.Verdict, []byte, error) {
	output, err := command.CombinedOutput()
	return evidence.Verdict{
		Command: spec.Gate,
		Code:    command.ProcessState.ExitCode(),
		Passed:  err == nil,
	}, output, nil
}

// Gate asks, then keeps the answer and the whole output.
func Gate(spec job.Spec, dir job.Dir) (evidence.Verdict, error) {
	return gateWith(spec, dir, nil, nil)
}

// gateWith is Gate with the environment the gate runs with, or this process's when env is nil, and behind a wall when wrap is given.
func gateWith(spec job.Spec, dir job.Dir, env []string, wrap func(*exec.Cmd)) (evidence.Verdict, error) {
	verdict, output, err := askIn(spec, dir, env, wrap)
	if err != nil {
		return evidence.Verdict{}, err
	}
	return verdict, keepGate(dir, verdict, output)
}

// keepGate writes the gate's answer and its whole output into the record.
func keepGate(dir job.Dir, verdict evidence.Verdict, output []byte) error {
	if err := os.WriteFile(dir.OutFile("gate.log"), output, 0o644); err != nil {
		return err
	}
	text, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile("gates.json"), append(text, '\n'), 0o644)
}
