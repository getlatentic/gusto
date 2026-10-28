package evidence

import (
	"encoding/json"
	"os"

	"github.com/getlatentic/gusto/pkg/job"
)

// A turn's record names who and what it was for, and what was done with what it changed, as well as what it did: the run's
// part of the audit getlatentic/planning#731 asks for. Both are files in out/, so the manifest hashes them and its signature
// covers them.

// RunFile and DecisionFile are where the two are kept in out/.
const (
	RunFile      = "run.json"
	DecisionFile = "decision.json"
)

// Run is who and where a run was for, as the server knew it when the run began.
type Run struct {
	// Asked is the person the run acts for, and PaidBy the one whose computer ran it and whose spending it counts against.
	Asked  string `json:"asked,omitempty"`
	PaidBy string `json:"paidBy,omitempty"`
	// Agent is the room's agent, by its id in the room and its name, and Room the room it worked in.
	Agent     string `json:"agent,omitempty"`
	AgentName string `json:"agentName,omitempty"`
	Room      string `json:"room,omitempty"`
	// Sandbox is the sandbox the agent ran in, by the name the gateway knows it by.
	Sandbox string `json:"sandbox,omitempty"`
	// AllowanceUSD is what the agent may spend in the room in a month, when its owner set it.
	AllowanceUSD *float64 `json:"allowanceUsd,omitempty"`
	// ModelToken is how the run was given its token for the model: TokenBound or TokenPlain.
	ModelToken string `json:"modelToken,omitempty"`
}

// A bound token works only with proofs from a key held in the run's sandbox; a plain token works for whoever holds it.
const (
	TokenBound = "bound"
	TokenPlain = "plain"
)

// Decision is what was done with what a room's turn changed, once it finished.
type Decision struct {
	// Kept says what the turn changed reached the room's folder. Refused is why it did not, when it did not.
	Kept    bool   `json:"kept"`
	Refused string `json:"refused,omitempty"`
	// Changed is how many files of the room it changed, and Joined and Conflicts the files it shared with another change.
	Changed   int      `json:"changed,omitempty"`
	Joined    []string `json:"joined,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
}

// WriteRun keeps who and where a run is for, before it starts.
func WriteRun(dir job.Dir, run Run) error { return writeOut(dir, RunFile, run) }

// Decide keeps what was done with what a room's turn changed, and puts it under the record's signature: the manifest is made
// again over out/ as it now is and signed again with the key the run was, when one is configured.
func Decide(dir job.Dir, decision Decision) error {
	if err := writeOut(dir, DecisionFile, decision); err != nil {
		return err
	}
	manifest, err := Read(dir)
	if err != nil {
		return err
	}
	if manifest.Job, manifest.Files, err = Of(dir); err != nil {
		return err
	}
	if err := Write(dir, manifest); err != nil {
		return err
	}
	return SealAsConfigured(dir)
}

func writeOut(dir job.Dir, name string, value any) error {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile(name), append(text, '\n'), 0o644)
}
