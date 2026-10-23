// Package compare runs one task on several agents and keeps the runs as one
// comparison.
//
// Every run is an ordinary task directory made from the same submission, and the
// runs differ in the agent alone: the brief, the check, the budget, the model
// when one is named, and the commit, pinned once before any run starts. Each
// run has its own workspace and its own record, and a container mounts only
// its own run, so no agent can see what another did. The comparison is the
// directory above the runs, and comparison.json in it names them.
package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/replay"
	"github.com/getlatentic/gusto/pkg/job"
)

// Prepare makes a comparison of submission across agents under parent, with
// a task directory ready to run for each agent, and returns where it is.
func Prepare(submission job.Dir, parent string, harnesses []string) (string, error) {
	if err := distinct(harnesses); err != nil {
		return "", err
	}
	spec, err := job.Read(submission)
	if err != nil {
		return "", err
	}
	if spec.Session != "" {
		return "", fmt.Errorf("%s is a turn in a conversation, which has no checkout of its own to compare on", submission)
	}
	where, err := claim(submission, parent)
	if err != nil {
		return "", err
	}
	if err := fill(submission, where, harnesses); err != nil {
		return "", errors.Join(err, os.RemoveAll(where))
	}
	return where, nil
}

// fill makes a run for each agent, pins them all to one commit, and writes
// the record naming them.
func fill(submission job.Dir, where string, harnesses []string) error {
	runs := make([]Run, 0, len(harnesses))
	for _, name := range harnesses {
		run := Run{Harness: name, Dir: name}
		if err := made(submission, run.Job(where), name); err != nil {
			return err
		}
		runs = append(runs, run)
	}
	pin, err := pinned(runs[0].Job(where))
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := replay.Rewrite(run.Job(where), run.Job(where), map[string]any{"ref": pin.Commit}); err != nil {
			return err
		}
	}
	held, err := shared(runs[0].Job(where))
	if err != nil {
		return err
	}
	of, err := filepath.Abs(submission.Path())
	if err != nil {
		return err
	}
	return writeRecord(where, Record{Submission: of, Commit: pin.Commit, PinnedBy: pin.By, Held: held, Runs: runs})
}

// made is the submission as a task for one agent: its job.json with the
// agent changed, and the adapter to run it with.
func made(submission, dir job.Dir, harness string) error {
	if err := dir.Make(); err != nil {
		return err
	}
	if err := replay.Rewrite(submission, dir, map[string]any{"harness": harness}); err != nil {
		return err
	}
	return replay.Carry(submission, dir, harness)
}

// pinned resolves the ref once, before any run clones. Each run resolving it on
// its own would let a branch that moved between the two clones hand the
// agents different code.
func pinned(first job.Dir) (replay.Pin, error) {
	spec, err := job.Read(first)
	if err != nil {
		return replay.Pin{}, err
	}
	return replay.Resolve(spec.Repo, spec.Ref)
}

// shared is every field in a run's job.json except the agent. The runs were
// all written from one submission with only the agent set, so what one run
// holds, every run holds.
func shared(run job.Dir) ([]string, error) {
	fields, err := fieldsOf(run)
	if err != nil {
		return nil, err
	}
	held := make([]string, 0, len(fields))
	for name := range fields {
		if name != "harness" {
			held = append(held, name)
		}
	}
	sort.Strings(held)
	return held, nil
}

// distinct refuses fewer than two agents, or one named twice: either would
// be a comparison of an agent with itself, and two runs cannot share a
// directory.
func distinct(harnesses []string) error {
	if len(harnesses) < 2 {
		return errors.New("a comparison needs at least two agents")
	}
	seen := map[string]bool{}
	for _, name := range harnesses {
		if seen[name] || name == "" || name != filepath.Base(name) {
			return fmt.Errorf("%q is named twice or is not an agent name", name)
		}
		seen[name] = true
	}
	return nil
}

// claim makes the comparison's directory under parent, beside the submission
// by default, numbered so a second comparison never lands in the first.
func claim(submission job.Dir, parent string) (string, error) {
	if parent == "" {
		parent = filepath.Dir(filepath.Clean(submission.Path()))
	}
	base := filepath.Base(filepath.Clean(submission.Path()))
	for n := 1; ; n++ {
		path := filepath.Join(parent, fmt.Sprintf("%s-compare-%d", base, n))
		err := os.Mkdir(path, 0o755)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}

func fieldsOf(dir job.Dir) (map[string]json.RawMessage, error) {
	text, err := os.ReadFile(dir.Spec())
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	err = json.Unmarshal(text, &fields)
	return fields, err
}
