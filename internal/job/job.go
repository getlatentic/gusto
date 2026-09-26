// Package job is the directory a run happens in, and the spec at its root.
//
// Every part of a run reads and writes this one directory, and nothing else
// passes between them. That is what makes a runner replaceable: hand the same
// directory to a managed fleet, to a container in the customer's own account,
// or to a laptop, and the work is the same.
package job

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Sensitivity is how sensitive the code in a job is. A runner says whose
// machine it is, and the two are matched before anything starts.
type Sensitivity string

const (
	Public     Sensitivity = "public"
	Internal   Sensitivity = "internal"
	Restricted Sensitivity = "restricted"
)

// Known reports whether this is a label the rules understand.
//
// An unknown label must never read as "not restricted". The rules ask whether a
// job is restricted, so `secret`, `confidential` or `Restricted` would each
// switch off the one rule this platform enforces, silently, by a typo.
func (s Sensitivity) Known() bool {
	switch s {
	case Public, Internal, Restricted:
		return true
	}
	return false
}

// Budget is what a job may spend before a runner stops it.
//
// USD is a cap on what the job's calls to a model may cost, counted at the
// proxy. Nil is no cap, and zero allows nothing, so leaving it out can never
// be mistaken for a cap of nothing or the other way round.
type Budget struct {
	Turns   int      `json:"turns"`
	Seconds int      `json:"seconds"`
	USD     *float64 `json:"usd,omitempty"`
}

// Spec carries a brief, not a prompt. The brief is input to a harness, and a
// field that passed a prompt straight through would make this a proxy for
// somebody else's model account.
//
// Version is the format of job.json. A file without one is version 1.
type Spec struct {
	Version int      `json:"version"`
	Brief   string   `json:"brief"`
	Repo    string   `json:"repo"`
	Ref     string   `json:"ref"`
	Gate    []string `json:"gate"`
	Harness string   `json:"harness"`
	Model   string   `json:"model,omitempty"`
	// Session names a conversation this job is a turn in. A job with one mounts
	// what stays instead of cloning, and Remembered is the harness's own name
	// for the conversation, handed back so it resumes rather than restarts.
	Session     string      `json:"session,omitempty"`
	Remembered  string      `json:"remembered,omitempty"`
	Sensitivity Sensitivity `json:"sensitivity"`
	Budget      Budget      `json:"budget"`
	// Team is who a job's spend counts against. It is a label the person
	// asking chooses: nothing proves they belong to the team yet.
	Team string `json:"team,omitempty"`
}

// Dir is a job directory.
type Dir string

func (d Dir) Path() string            { return string(d) }
func (d Dir) Spec() string            { return filepath.Join(string(d), "job.json") }
func (d Dir) Workspace() string       { return filepath.Join(string(d), "workspace") }
func (d Dir) Out() string             { return filepath.Join(string(d), "out") }
func (d Dir) OutFile(n string) string { return filepath.Join(d.Out(), n) }

func (d Dir) Make() error {
	if err := os.MkdirAll(d.Workspace(), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(d.Out(), 0o755)
}

// Read the spec, filling in what a person left out. A field this build does not
// know is ignored, so a job written after an additive change still runs; a
// version it does not know is refused.
func Read(d Dir) (Spec, error) {
	text, err := os.ReadFile(d.Spec())
	if err != nil {
		return Spec{}, err
	}
	var spec Spec
	if err := json.Unmarshal(text, &spec); err != nil {
		return Spec{}, err
	}
	if err := Readable("job.json", spec.Version, Version); err != nil {
		return Spec{}, err
	}
	spec = spec.withDefaults()
	if !spec.Sensitivity.Known() {
		return Spec{}, fmt.Errorf("%q is not a sensitivity this runner knows, so it cannot say what may leave", spec.Sensitivity)
	}
	if err := spec.Budget.Check(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// Check refuses a money cap nothing could spend under. A negative cap would
// refuse every call and read, to whoever set it, like a typo that capped nothing.
func (b Budget) Check() error {
	if b.USD != nil && (*b.USD < 0 || math.IsNaN(*b.USD) || math.IsInf(*b.USD, 0)) {
		return fmt.Errorf("a budget of $%.4f is not a cap anything can spend under", *b.USD)
	}
	return nil
}

func (s Spec) withDefaults() Spec {
	if s.Version == 0 {
		s.Version = Version
	}
	if s.Ref == "" {
		s.Ref = "HEAD"
	}
	if s.Sensitivity == "" {
		s.Sensitivity = Internal
	}
	if s.Budget.Turns == 0 {
		s.Budget.Turns = 40
	}
	if s.Budget.Seconds == 0 {
		s.Budget.Seconds = 900
	}
	return s
}

func (s Spec) Write(d Dir) error {
	text, err := json.MarshalIndent(s.withDefaults(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.Spec(), append(text, '\n'), 0o644)
}
