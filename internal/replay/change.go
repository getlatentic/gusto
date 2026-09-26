package replay

// Change is what happened to a verdict between the original and its replay.
type Change string

const (
	Same   Change = "same"
	Worse  Change = "worse"
	Better Change = "better"
	// Unknown is a replay that gave no verdict, so nothing can be said.
	Unknown Change = "unknown"
)

// Change compares the gate's answers. Passing is what the gate decides, so a
// job that passed and now fails got worse, whatever the harness says about it.
// An exit code that differs while the pass stays the same is shown in the
// summary and is not a change in the verdict.
func (r Row) Change() Change {
	if r.Err != nil {
		return Unknown
	}
	before, after := r.Before.Gate.Passed, r.After.Gate.Passed
	switch {
	case before && !after:
		return Worse
	case !before && after:
		return Better
	}
	return Same
}

// ExitCode says how the sweep went in one number: 1 when any verdict got worse,
// 2 when any job could not be replayed and none got worse, 0 otherwise. A
// worse verdict wins over an error because it is the answer the sweep was run
// to find, and either one is non-zero.
func ExitCode(rows []Row) int {
	counts := tally(rows)
	switch {
	case counts[Worse] > 0:
		return 1
	case counts[Unknown] > 0:
		return 2
	}
	return 0
}

func tally(rows []Row) map[Change]int {
	counts := map[Change]int{}
	for _, row := range rows {
		counts[row.Change()]++
	}
	return counts
}
