package matrix

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/getlatentic/haas/internal/job"
)

// Plan is what a person asks the matrix to measure: a fixed set of jobs, and
// the harness and model pairs to run each of them on.
type Plan struct {
	// Environment names the place the runs happen, such as "laptop" or "box".
	// The same harness and model cost twenty times as much on a laptop that
	// carries its whole skill set into every request as in a container that
	// starts empty, so a row that does not say where it ran cannot be read.
	Environment string `json:"environment,omitempty"`
	// Jobs are job directories as `haas new` leaves them, relative to the
	// plan's own file.
	Jobs []string `json:"jobs"`
	// Simplest is the job a pair has to finish to be offered at all. It is the
	// first job when the plan leaves it out.
	Simplest string `json:"simplest,omitempty"`
	Pairs    []Pair `json:"pairs"`
}

// Pair is one harness and the model it is asked to think with. A pair with no
// model runs whatever model the job names, or the harness's own default.
type Pair struct {
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
}

// Name is how a person writes the pair on the command line and reads it in a
// table: the harness, then the model after an at sign.
func (p Pair) Name() string {
	if p.Model == "" {
		return p.Harness
	}
	return p.Harness + "@" + p.Model
}

// Dir is the run's directory name. A model name carries slashes and colons,
// and a colon in a directory breaks a container mount, so anything but letters,
// digits and . _ @ - becomes a dash.
func (p Pair) Dir() string {
	return unsafeInDir.ReplaceAllString(p.Name(), "-")
}

var unsafeInDir = regexp.MustCompile(`[^A-Za-z0-9._@-]`)

// ReadPlan reads a plan and makes its job directories absolute, so the matrix
// finds them wherever it is run from.
func ReadPlan(path string) (Plan, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(text, &plan); err != nil {
		return Plan{}, fmt.Errorf("%s is not a plan: %w", path, err)
	}
	beside, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Plan{}, err
	}
	for i, named := range plan.Jobs {
		if !filepath.IsAbs(named) {
			plan.Jobs[i] = filepath.Join(beside, named)
		}
	}
	return plan, plan.check()
}

// check refuses a plan whose runs would share a directory or have nothing to
// measure, before anything is cloned or anybody's money is spent.
func (p Plan) check() error {
	if len(p.Jobs) == 0 || len(p.Pairs) == 0 {
		return errors.New("a plan needs at least one job and one harness and model pair")
	}
	if err := distinct(p.Jobs, JobName); err != nil {
		return err
	}
	if err := distinct(p.Pairs, Pair.Dir); err != nil {
		return err
	}
	for _, pair := range p.Pairs {
		if pair.Harness == "" || pair.Harness != filepath.Base(pair.Harness) {
			return fmt.Errorf("%q is not a harness name", pair.Harness)
		}
	}
	if p.Simplest != "" && !contains(p.Jobs, p.Simplest) {
		return fmt.Errorf("the simplest job, %s, is not one of the plan's jobs", p.Simplest)
	}
	return nil
}

// SimplestJob is the name of the job a pair has to finish to be offered.
func (p Plan) SimplestJob() string {
	if p.Simplest != "" {
		return p.Simplest
	}
	return JobName(p.Jobs[0])
}

// JobName is what a job is called in the matrix: its directory's own name.
func JobName(path string) string {
	return filepath.Base(filepath.Clean(path))
}

// submissions reads each job, refusing a turn in a conversation, which has no
// checkout of its own to measure on.
func (p Plan) submissions() ([]job.Dir, error) {
	dirs := make([]job.Dir, 0, len(p.Jobs))
	for _, path := range p.Jobs {
		spec, err := job.Read(job.Dir(path))
		if err != nil {
			return nil, err
		}
		if spec.Session != "" {
			return nil, fmt.Errorf("%s is a turn in a conversation, which has no checkout of its own to measure on", path)
		}
		dirs = append(dirs, job.Dir(path))
	}
	return dirs, nil
}

func distinct[T any](items []T, name func(T) string) error {
	seen := map[string]bool{}
	for _, item := range items {
		key := name(item)
		if seen[key] {
			return fmt.Errorf("%s is named twice, and two runs cannot share a directory", key)
		}
		seen[key] = true
	}
	return nil
}

func contains(paths []string, name string) bool {
	for _, path := range paths {
		if JobName(path) == name {
			return true
		}
	}
	return false
}
