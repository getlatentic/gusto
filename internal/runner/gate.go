package runner

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// Ask runs the command that says pass or fail and writes nothing. A harness
// reporting success is a claim; this is the check, and a re-run of it has to
// leave the evidence it is checking untouched.
func Ask(spec job.Spec, dir job.Dir) (evidence.Verdict, []byte, error) {
	if len(spec.Gate) == 0 {
		return evidence.Verdict{}, nil, errors.New("the job names no gate")
	}
	command := exec.Command(spec.Gate[0], spec.Gate[1:]...)
	command.Dir = dir.Workspace()
	output, err := command.CombinedOutput()
	return evidence.Verdict{
		Command: spec.Gate,
		Code:    command.ProcessState.ExitCode(),
		Passed:  err == nil,
	}, output, nil
}

// Gate asks, then keeps the answer and the whole output.
func Gate(spec job.Spec, dir job.Dir) (evidence.Verdict, error) {
	verdict, output, err := Ask(spec, dir)
	if err != nil {
		return evidence.Verdict{}, err
	}
	if err := os.WriteFile(dir.OutFile("gate.log"), output, 0o644); err != nil {
		return evidence.Verdict{}, err
	}
	text, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return evidence.Verdict{}, err
	}
	return verdict, os.WriteFile(dir.OutFile("gates.json"), append(text, '\n'), 0o644)
}
