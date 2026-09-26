package compare

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Print writes the comparison side by side: where it is, what the runs share,
// then a line a harness.
func Print(w io.Writer, where string, record Record, sides []Side) {
	fmt.Fprintf(w, "comparison  %s\ncommit      %s\nheld        %s\n\n", where, record.Commit, strings.Join(record.Held, ", "))
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "harness\tverdict\tharness exit\tat budget\ttokens\tcost\tevidence")
	for _, side := range sides {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", side.Harness, verdict(side), exit(side),
			budget(side), tokens(side), cost(side), checks(side))
	}
	table.Flush()
	for _, side := range sides {
		if side.Err != nil {
			fmt.Fprintf(w, "%s: %v\n", side.Harness, side.Err)
		}
	}
}

func verdict(side Side) string {
	switch gate := side.Manifest.Gate; {
	case gate == nil:
		return "-"
	case gate.Passed:
		return fmt.Sprintf("passed (%d)", gate.Code)
	default:
		return fmt.Sprintf("failed (%d)", gate.Code)
	}
}

func exit(side Side) string {
	if side.Manifest.Gate == nil {
		return "-"
	}
	return fmt.Sprint(side.Manifest.HarnessExit)
}

func budget(side Side) string {
	if side.Manifest.Gate == nil {
		return "-"
	}
	return fmt.Sprint(side.Manifest.StoppedAtBudget)
}

// tokens and cost print a dash where the harness reported nothing, the way the
// manifest keeps a null: a zero would read as free.
func tokens(side Side) string {
	if side.Manifest.Tokens == nil {
		return "-"
	}
	return fmt.Sprint(*side.Manifest.Tokens)
}

func cost(side Side) string {
	if side.Manifest.CostUSD == nil {
		return "-"
	}
	return fmt.Sprintf("$%.4f", *side.Manifest.CostUSD)
}

func checks(side Side) string {
	switch {
	case side.Err != nil:
		return "-"
	case len(side.Wrong) > 0:
		return side.Wrong[0]
	}
	return "checks"
}
