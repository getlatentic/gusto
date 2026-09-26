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
type Budget struct {
	Turns   int `json:"turns"`
	Seconds int `json:"seconds"`
}

// Spec carries a brief, not a prompt. The brief is input to a harness, and a
// field that passed a prompt straight through would make this a proxy for
// somebody else's model account.
type Spec struct {
	Brief       string      `json:"brief"`
	Repo        string      `json:"repo"`
	Ref         string      `json:"ref"`
	Gate        []string    `json:"gate"`
	Harness     string      `json:"harness"`
	Model       string      `json:"model,omitempty"`
	Sensitivity Sensitivity `json:"sensitivity"`
	Budget      Budget      `json:"budget"`
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

// Read the spec, filling in what a person left out.
func Read(d Dir) (Spec, error) {
	text, err := os.ReadFile(d.Spec())
	if err != nil {
		return Spec{}, err
	}
	var spec Spec
	if err := json.Unmarshal(text, &spec); err != nil {
		return Spec{}, err
	}
	spec = spec.withDefaults()
	if !spec.Sensitivity.Known() {
		return Spec{}, fmt.Errorf("%q is not a sensitivity this runner knows, so it cannot say what may leave", spec.Sensitivity)
	}
	return spec, nil
}

func (s Spec) withDefaults() Spec {
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
