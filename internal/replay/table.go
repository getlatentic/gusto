package replay

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/getlatentic/gusto/internal/record"
)

// Print writes the sweep's one summary: a line a task, then the totals, then
// why each task that could not be rerun could not.
func Print(w io.Writer, rows []Row) {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "task\tagent\tbefore\tafter\tagent exit\tchange\trerun")
	for _, row := range rows {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Old, harnesses(row), verdict(row.Before), verdict(row.After),
			exits(row), row.Change(), or(row.Replay.Path(), "-"))
	}
	table.Flush()
	counts := tally(rows)
	fmt.Fprintf(w, "\n%d rerun: %d worse, %d better, %d same, %d could not run\n",
		len(rows), counts[Worse], counts[Better], counts[Same], counts[Unknown])
	for _, row := range rows {
		if row.Err != nil {
			fmt.Fprintf(w, "%s: %v\n", row.Old, row.Err)
		}
	}
}

func verdict(manifest record.Manifest) string {
	if manifest.Check == nil {
		return "-"
	}
	if manifest.Check.Passed {
		return fmt.Sprintf("passed (%d)", manifest.Check.Code)
	}
	return fmt.Sprintf("failed (%d)", manifest.Check.Code)
}

// harnesses names one harness when both runs used it, which is what a replay
// means, and both when they differ, which is worth seeing.
func harnesses(row Row) string {
	before, after := or(row.Before.Harness, "-"), row.After.Harness
	if after == "" || after == before {
		return before
	}
	return before + " -> " + after
}

func exits(row Row) string {
	if row.After.Check == nil {
		return fmt.Sprintf("%d -> -", row.Before.HarnessExit)
	}
	return fmt.Sprintf("%d -> %d", row.Before.HarnessExit, row.After.HarnessExit)
}

func or(value, otherwise string) string {
	if value != "" {
		return value
	}
	return otherwise
}
