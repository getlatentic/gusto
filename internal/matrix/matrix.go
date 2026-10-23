// Package matrix measures which agent and model pairs can finish a job.
//
// A plan names a fixed set of tasks and the pairs to run each on. Every task
// becomes a comparison in the shape internal/compare keeps, pinned once to one
// commit, with one ordinary task directory a pair, so every row of the table is
// a run whose record can be verified, sent as a pull request or rerun like any other.
// The benchmark is the directory above the comparisons, and matrix.json in it
// says where the runs happened and how long each one took.
//
// Each run costs money, so a run that already finished is never run again
// unless it is asked for, and a benchmark stopped midway picks up where it was.
package matrix

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/replay"
	"github.com/getlatentic/gusto/pkg/job"
)

// Again says whether a pair is run again although it already finished.
type Again func(Pair) bool

// AgainFor reads -again: "all", or pairs by name, agent@model, separated by
// commas. Nothing named runs nothing again.
func AgainFor(named string) Again {
	wanted := map[string]bool{}
	for _, name := range strings.Split(named, ",") {
		wanted[strings.TrimSpace(name)] = true
	}
	return func(pair Pair) bool { return wanted["all"] || wanted[pair.Name()] }
}

// Measure makes every run of the plan that has not finished into the benchmark at
// where, one at a time, and returns the benchmark as it stands.
//
// The runs go one after another rather than together. The wall clock is one
// of the answers, and two runs sharing a machine or a local model would each
// be measured slowing the other down.
func Measure(plan Plan, where string, way Way, again Again, progress io.Writer) (Record, error) {
	submissions, err := plan.submissions()
	if err != nil {
		return Record{}, err
	}
	record, err := opened(where, environmentOf(plan.Environment, way.Where))
	if err != nil {
		return Record{}, err
	}
	record.Simplest = plan.SimplestJob()
	for _, submission := range submissions {
		column := column{where: where, record: &record, submission: submission, pairs: plan.Pairs}
		if err := column.measure(way, again, progress); err != nil {
			return record, err
		}
	}
	return record, nil
}

// environmentOf is the plan's name for the place and how the runs start there.
func environmentOf(named, where string) string {
	if named == "" {
		return where
	}
	return named + ", " + where
}

// column is one job and every pair it is run on.
type column struct {
	where      string
	record     *Record
	submission job.Dir
	pairs      []Pair
}

func (c column) name() string { return JobName(c.submission.Path()) }

func (c column) root() string { return filepath.Join(c.where, c.name()) }

// measure readies the job's comparison, then runs each pair that is due.
func (c column) measure(way Way, again Again, progress io.Writer) error {
	comparison, err := prepared(c.root(), c.submission, c.pairs)
	if err != nil {
		return err
	}
	due := c.due(again)
	if err := write(c.where, *c.record); err != nil || len(due) == 0 {
		return err
	}
	run, stop, err := way.Open(c.root())
	if err != nil {
		return err
	}
	defer stop()
	for _, pair := range due {
		fmt.Fprintf(progress, "running %s on %s\n", c.name(), pair.Name())
		if err := c.runOne(pair, comparison.Commit, run, progress); err != nil {
			return err
		}
	}
	return nil
}

// due is every pair whose cell has not finished, and every one asked for again.
func (c column) due(again Again) []Pair {
	var due []Pair
	for _, pair := range c.pairs {
		cell := c.record.cell(c.name(), pair)
		if again(pair) || !Finished(cell.Run(c.where)) {
			due = append(due, pair)
		}
	}
	return due
}

// runOne runs one cell from a clean directory and times it. A run that fails is
// said and left for the next time the matrix runs; only a record that cannot
// be written stops the matrix.
func (c column) runOne(pair Pair, commit string, run replay.Run, progress io.Writer) error {
	dir := c.record.cell(c.name(), pair).Run(c.where)
	if err := c.clean(dir, pair, commit); err != nil {
		return err
	}
	started := time.Now()
	failed := run(dir)
	took := time.Since(started).Seconds()
	if failed != nil || !Finished(dir) {
		fmt.Fprintf(progress, "%s on %s left no verdict: %v\n", c.name(), pair.Name(), failed)
		return nil
	}
	cell := c.record.cell(c.name(), pair)
	cell.StartedUnix, cell.WallSeconds = started.Unix(), &took
	return write(c.where, *c.record)
}

// clean gives the cell a directory nothing has run in. An earlier attempt is
// moved aside rather than deleted, because it is evidence of what that run
// did, and remade from the submission at the comparison's commit.
func (c column) clean(dir job.Dir, pair Pair, commit string) error {
	if !used(dir) {
		return nil
	}
	if err := retire(dir); err != nil {
		return err
	}
	if err := made(c.submission, dir, pair); err != nil {
		return err
	}
	return replay.Rewrite(dir, dir, map[string]any{"ref": commit})
}

// Finished says whether the run at dir left the check's verdict.
func Finished(dir job.Dir) bool {
	manifest, err := evidence.Read(dir)
	return err == nil && manifest.Gate != nil
}

// used is a run directory something has already cloned into or written to.
func used(dir job.Dir) bool {
	for _, path := range []string{dir.Workspace(), dir.Out()} {
		if listing, err := os.ReadDir(path); err == nil && len(listing) > 0 {
			return true
		}
	}
	return false
}

// retire moves an earlier attempt beside the run, numbered, where no
// comparison names it.
func retire(dir job.Dir) error {
	for n := 1; ; n++ {
		aside := fmt.Sprintf("%s.attempt-%d", dir.Path(), n)
		_, err := os.Lstat(aside)
		if errors.Is(err, os.ErrNotExist) {
			return os.Rename(dir.Path(), aside)
		}
		if err != nil {
			return err
		}
	}
}
