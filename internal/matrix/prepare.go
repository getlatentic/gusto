package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/replay"
)

// A job in the matrix is a comparison, in the shape `gusto compare` writes and
// reads: comparison.json above one ordinary job directory a pair, every one
// pinned to one commit. So `gusto compare <matrix>/<job>` prints any column of
// the matrix, and a matrix is many comparisons side by side.

// prepared is the comparison for one job with a run for every pair, made
// where it was missing. A comparison made before keeps its commit, so a pair
// added to the plan later works on the same code as the pairs already run.
func prepared(where string, submission job.Dir, pairs []Pair) (compare.Record, error) {
	record, err := compare.Read(where)
	if errors.Is(err, os.ErrNotExist) {
		record, err = begun(where, submission)
	}
	if err != nil {
		return compare.Record{}, err
	}
	var added []compare.Run
	for _, pair := range pairs {
		if holds(record, pair.Dir()) {
			continue
		}
		run := compare.Run{Harness: pair.Harness, Dir: pair.Dir()}
		if err := made(submission, run.Job(where), pair); err != nil {
			return compare.Record{}, err
		}
		added = append(added, run)
	}
	if err := pinned(where, &record, added); err != nil {
		return compare.Record{}, err
	}
	record.Runs = append(record.Runs, added...)
	if record.Held, err = held(where, record.Runs); err != nil {
		return compare.Record{}, err
	}
	return record, writeComparison(where, record)
}

func begun(where string, submission job.Dir) (compare.Record, error) {
	of, err := filepath.Abs(submission.Path())
	if err != nil {
		return compare.Record{}, err
	}
	return compare.Record{Submission: of}, os.MkdirAll(where, 0o755)
}

// made is the submission as a job for one pair: its job.json with the harness,
// and the model when the pair names one, and the adapter to run it with.
func made(submission, dir job.Dir, pair Pair) error {
	if err := dir.Make(); err != nil {
		return err
	}
	set := map[string]any{"harness": pair.Harness}
	if pair.Model != "" {
		set["model"] = pair.Model
	}
	if err := replay.Rewrite(submission, dir, set); err != nil {
		return err
	}
	return replay.Carry(submission, dir, pair.Harness)
}

// pinned resolves the ref once, the first time the comparison gets a run, and
// writes the commit into every run added now. Resolving it again later would
// let a branch that moved hand a pair added later different code.
func pinned(where string, record *compare.Record, added []compare.Run) error {
	if len(added) == 0 {
		return nil
	}
	if record.Commit == "" {
		spec, err := job.Read(added[0].Job(where))
		if err != nil {
			return err
		}
		pin, err := replay.Resolve(spec.Repo, spec.Ref)
		if err != nil {
			return err
		}
		record.Commit, record.PinnedBy = pin.Commit, pin.By
	}
	for _, run := range added {
		if err := replay.Rewrite(run.Job(where), run.Job(where), map[string]any{"ref": record.Commit}); err != nil {
			return err
		}
	}
	return nil
}

// held is every job.json field whose value is the same in every run, read off
// the files. In a matrix the model usually differs, so it is held only when
// every pair asked for the same one.
func held(where string, runs []compare.Run) ([]string, error) {
	var first map[string]json.RawMessage
	same := map[string]bool{}
	for i, run := range runs {
		fields, err := fieldsOf(run.Job(where))
		if err != nil {
			return nil, err
		}
		if i == 0 {
			first = fields
			for name := range fields {
				same[name] = true
			}
			continue
		}
		for name := range same {
			same[name] = same[name] && equal(first[name], fields[name])
		}
	}
	names := make([]string, 0, len(same))
	for name, kept := range same {
		if kept {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func equal(a, b json.RawMessage) bool {
	var left, right bytes.Buffer
	if a == nil || b == nil || json.Compact(&left, a) != nil || json.Compact(&right, b) != nil {
		return false
	}
	return bytes.Equal(left.Bytes(), right.Bytes())
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

func holds(record compare.Record, dir string) bool {
	for _, run := range record.Runs {
		if run.Dir == dir {
			return true
		}
	}
	return false
}

// writeComparison writes compare's own record, at compare's version, so the
// comparison reads back through compare.Read like one `gusto compare` made.
func writeComparison(where string, record compare.Record) error {
	record.Version = compare.RecordVersion
	text, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(where, compare.RecordFile), append(text, '\n'), 0o644)
}
