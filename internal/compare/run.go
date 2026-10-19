package compare

import (
	"sync"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/replay"
)

// Side is one run as it stands: its verdict and whether its record still
// checks.
type Side struct {
	Run
	Manifest evidence.Manifest
	// Wrong lists what changed in the run's record since it ran.
	Wrong []string
	// Err is a run that could not happen or left no verdict.
	Err error
}

// Start runs every side of the comparison at where at once, so no agent
// waits on another and each meets the same hour of the day and the same model
// load. It returns what each run left.
func Start(where string, run replay.Run) ([]Side, error) {
	record, err := Read(where)
	if err != nil {
		return nil, err
	}
	failed := make([]error, len(record.Runs))
	var runs sync.WaitGroup
	for i, side := range record.Runs {
		runs.Add(1)
		go func() {
			defer runs.Done()
			failed[i] = run(side.Job(where))
		}()
	}
	runs.Wait()
	sides := Sides(where, record)
	for i := range sides {
		if failed[i] != nil {
			sides[i].Err = failed[i]
		}
	}
	return sides, nil
}

// Sides reads every run of the comparison from its own evidence.
func Sides(where string, record Record) []Side {
	sides := make([]Side, 0, len(record.Runs))
	for _, run := range record.Runs {
		sides = append(sides, sideOf(where, run))
	}
	return sides
}

func sideOf(where string, run Run) Side {
	side := Side{Run: run}
	dir := run.Job(where)
	side.Manifest, side.Err = evidence.Read(dir)
	if side.Err != nil {
		return side
	}
	side.Wrong, side.Err = evidence.Check(dir)
	return side
}

// ExitCode is 0 when every side ran, left a verdict and still checks, and 2
// otherwise. Which agent did better is the answer the comparison prints, not
// a failure of the comparison.
func ExitCode(sides []Side) int {
	for _, side := range sides {
		if side.Err != nil || side.Manifest.Gate == nil || len(side.Wrong) > 0 {
			return 2
		}
	}
	return 0
}
