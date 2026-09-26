package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/getlatentic/haas/internal/spend"
)

// spendReport says what the jobs a server keeps spent in a month, by team,
// project and job. It rebuilds the ledger from the jobs' own proxy records,
// through the same counting the server's caps use, so it needs no server.
func spendReport(args []string) int {
	flags := flag.NewFlagSet("spend", flag.ContinueOnError)
	root := flags.String("jobs", setting("HAAS_JOBS", "jobs"), "the directory the server keeps jobs in")
	month := flags.String("month", "", "the month, as 2006-01; this month when left out")
	asJSON := flags.Bool("json", false, "print the report as JSON")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	ledger, err := spend.Rebuild(*root, spend.Policy{})
	if err != nil {
		return fail(err)
	}
	if *month == "" {
		*month = ledger.ThisMonth()
	}
	report := ledger.Report(*month)
	if *asJSON {
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
