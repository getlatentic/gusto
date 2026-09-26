package spend

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Money is a tally's cost as a person should read it. A sum with unmeasured
// calls in it is a floor, and says so, because a total that quietly leaves them
// out reads as the whole bill.
func (t Tally) Money() string {
	switch {
	case t.Unmeasured == 0:
		return fmt.Sprintf("$%.4f", t.USD)
	case t.Calls == t.Unmeasured:
		return "unmeasured"
	}
	return fmt.Sprintf("at least $%.4f", t.USD)
}

// WriteText writes the report as three tables for a terminal.
func (r Report) WriteText(to io.Writer) error {
	table := tabwriter.NewWriter(to, 0, 4, 2, ' ', 0)
	fmt.Fprintf(table, "spend in %s\t%s\t%d calls, %d unmeasured, %d refused\n",
		r.Month, r.Total.Money(), r.Total.Calls, r.Total.Unmeasured, r.Total.Refused)
	for _, part := range []struct {
		title string
		lines []Line
	}{{"team", r.Teams}, {"project", r.Projects}, {"job", r.Jobs}} {
		fmt.Fprintf(table, "\n%s\tspent\tcalls\tunmeasured\trefused\tinput tokens\toutput tokens\n", part.title)
		for _, line := range part.lines {
			fmt.Fprintf(table, "%s\t%s\t%d\t%d\t%d\t%d\t%d\n", named(line.Name), line.Money(),
				line.Calls, line.Unmeasured, line.Refused, line.InputTokens, line.OutputTokens)
		}
	}
	return table.Flush()
}

// named is how a line with no name reads: a job that named no team.
func named(name string) string {
	if name == "" {
		return "(none)"
	}
	return name
}
