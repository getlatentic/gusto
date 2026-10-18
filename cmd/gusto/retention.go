package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/internal/retention"
)

type retentionFlags struct{ tasks, dir string }

func declareRetention(set *flag.FlagSet) *retentionFlags {
	f := &retentionFlags{}
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	set.StringVar(&f.dir, "dir", "", "the retention `directory`, with policy.json and the holds (default $GUSTO_RETENTION)")
	return f
}

// retentionCommand is `gusto retention`: it lists what a sweep would remove now,
// and removes nothing. The server sweeps on its own schedule; this is how an
// operator sees the effect of a policy before it applies.
func retentionCommand(args []string) int {
	set := newSet("retention")
	f := declareRetention(set)
	if code, ok := parseFlags("retention", set, args); !ok {
		return code
	}
	dir, tasksDir := orSetting(f.dir, "GUSTO_RETENTION", ""), orSetting(f.tasks, "GUSTO_JOBS", "jobs")
	if set.NArg() > 0 || dir == "" {
		return usageError("retention", "name the retention directory with -dir or GUSTO_RETENTION")
	}
	policy, err := retention.LoadPolicy(dir)
	if err != nil {
		return fail(err)
	}
	held, err := retention.OpenHolds(dir).Read()
	if err != nil {
		return fail(err)
	}
	report := retention.Sweep(retention.Scanned{Root: tasksDir}, tasksDir, policy, held, time.Now(), true)
	printSweep(report, filepath.Join(dir, retention.PolicyFile), policy)
	return 0
}

func printSweep(report retention.Report, path string, policy retention.Policy) {
	if !policy.Any() {
		fmt.Printf("%s removes nothing: it sets no days.\n", path)
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "task\tremoves\tstate\tended\tfrees")
	var total int64
	for _, action := range report.Actions {
		fmt.Fprintf(table, "%s\t%s\t%s\t%d days ago\t%d bytes\n", action.Job, action.Kind, action.State, action.Days, action.Bytes)
		total += action.Bytes
	}
	table.Flush()
	fmt.Printf("%d to remove, freeing %d bytes; %d kept by a hold. Nothing was removed.\n", len(report.Actions), total, len(report.Held))
}
