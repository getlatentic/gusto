package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/haas/internal/retention"
)

// retentionCommand is `haas retention`: it lists what a sweep would remove now,
// and removes nothing. The server sweeps on its own schedule; this is how an
// operator sees the effect of a policy before it applies.
func retentionCommand(args []string) int {
	flags := flag.NewFlagSet("retention", flag.ContinueOnError)
	jobsDir := flags.String("jobs", setting("HAAS_JOBS", "jobs"), "the directory the server keeps jobs in")
	dir := flags.String("dir", os.Getenv("HAAS_RETENTION"), "the retention directory, with policy.json and the holds")
	if err := flags.Parse(args); err != nil || flags.NArg() > 0 || *dir == "" {
		fmt.Fprintln(os.Stderr, "haas retention: name the retention directory with -dir or HAAS_RETENTION")
		return 2
	}
	policy, err := retention.LoadPolicy(*dir)
	if err != nil {
		return fail(err)
	}
	held, err := retention.OpenHolds(*dir).Read()
	if err != nil {
		return fail(err)
	}
	report := retention.Sweep(retention.Scanned{Root: *jobsDir}, *jobsDir, policy, held, time.Now(), true)
	printSweep(report, filepath.Join(*dir, retention.PolicyFile), policy)
	return 0
}

func printSweep(report retention.Report, path string, policy retention.Policy) {
	if !policy.Any() {
		fmt.Printf("%s removes nothing: it sets no days.\n", path)
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "job\tremoves\tstate\tended\tfrees")
	var total int64
	for _, action := range report.Actions {
		fmt.Fprintf(table, "%s\t%s\t%s\t%d days ago\t%d bytes\n", action.Job, action.Kind, action.State, action.Days, action.Bytes)
		total += action.Bytes
	}
	table.Flush()
	fmt.Printf("%d to remove, freeing %d bytes; %d kept by a hold. Nothing was removed.\n", len(report.Actions), total, len(report.Held))
}
