package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/getlatentic/gusto/pkg/spend"
)

type spendFlags struct {
	tasks, month string
	json         bool
}

func declareSpend(set *flag.FlagSet) *spendFlags {
	f := &spendFlags{}
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	set.StringVar(&f.month, "month", "", "the month, as `2006-01`; this month when left out")
	set.BoolVar(&f.json, "json", false, "print the report as JSON")
	return f
}

// spendReport says what the tasks a server keeps spent in a month, by team,
// project and task. It rebuilds the ledger from the tasks' own proxy records,
// through the same counting the server's caps use, so it needs no server.
func spendReport(args []string) int {
	set := newSet("spend")
	f := declareSpend(set)
	if code, ok := parseFlags("spend", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("spend", "takes no arguments")
	}
	ledger, err := spend.Rebuild(orSetting(f.tasks, "GUSTO_JOBS", "jobs"), spend.Policy{})
	if err != nil {
		return fail(err)
	}
	if f.month == "" {
		f.month = ledger.ThisMonth()
	}
	report := ledger.Report(f.month)
	if f.json {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(report)
	} else {
		err = report.WriteText(os.Stdout)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}
