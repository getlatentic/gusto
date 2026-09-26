package doctor

import (
	"fmt"
	"io"
)

// Print writes one line a check, and under a failure what to do about it, so
// the line that failed and its remedy are read together.
func Print(to io.Writer, results []Result) {
	width := widest(results)
	for _, result := range results {
		fmt.Fprintf(to, "%-4s  %-*s  %s\n", result.Status, width, result.Name, result.Said)
		if result.Remedy != "" {
			fmt.Fprintf(to, "%-4s  %-*s  fix: %s\n", "", width, "", result.Remedy)
		}
	}
}

func widest(results []Result) int {
	width := 0
	for _, result := range results {
		width = max(width, len(result.Name))
	}
	return width
}
