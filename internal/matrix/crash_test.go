package matrix_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/matrix"
	"github.com/getlatentic/gusto/internal/replay"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/task"
)

// A stop after an earlier attempt was moved aside, and before the cell was made again, leaves no directory: the next measure
// makes it and runs it. spec/Matrix.before-remake.cfg is the trace.
func TestACellMovedAsideAndNotMadeAgainIsMadeAndRun(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes)
	var ran int
	where, _ := measure(t, path, "", counting(&ran))
	cell := filepath.Join(where, "one-line-fix", finishes.Dir())
	if err := os.Rename(cell, cell+".attempt-1"); err != nil {
		t.Fatal(err)
	}
	_, table := measure(t, path, "", counting(&ran))
	if row := rowFor(t, table, "one-line-fix", finishes); row.Finished == nil || !*row.Finished {
		t.Errorf("the cell was not made and run again: %+v", row)
	}
}

// A run that left its verdict and then failed is timed, and a run again does not keep the time of the run it replaced.
// spec/Matrix.before-time.cfg is the trace.
func TestACellsTimeIsTheTimeOfTheRunItHolds(t *testing.T) {
	hermetic(t)
	path := planned(t, remote(t), finishes)
	var ran int
	_, table := measure(t, path, "", counting(&ran))
	first := rowFor(t, table, "one-line-fix", finishes).WallSeconds
	if first == nil {
		t.Fatal("the first run was not timed")
	}
	afterwards := matrix.Way{Where: "here, in this process", Open: func(string) (replay.Run, func(), error) {
		return func(dir task.Dir) error {
			return errors.Join(replay.Here(signing.Identity{})(dir), errors.New("the runner failed after the verdict"))
		}, func() {}, nil
	}}
	_, table = measure(t, path, finishes.Name(), afterwards)
	again := rowFor(t, table, "one-line-fix", finishes).WallSeconds
	if again == nil {
		t.Fatal("the run again that left a verdict is not timed")
	}
	if *again == *first {
		t.Errorf("the run again is timed %v, the time of the run it replaced", *again)
	}
}
