package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/getlatentic/haas/internal/audit"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// exportJob writes a job's audit export, or with -verify checks one. It is
// the verb's only identifier in this package; everything it does is in
// internal/audit.
func exportJob(args []string) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	to := flags.String("to", "", "the new directory to write the export into (default <dir>.audit)")
	policyFile := flags.String("policy", "", "a JSON file saying what the export drops and redacts")
	requestLog := flags.String("requests", "", "the server's request log, one JSON line a request")
	check := flags.Bool("verify", false, "check an export instead of writing one")
	where, rest := split(args)
	if err := flags.Parse(rest); err != nil {
		return 2
	}
	if where == "" && flags.NArg() == 1 {
		where = flags.Arg(0)
	}
	if where == "" {
		fmt.Fprintln(os.Stderr, "haas export needs a job directory, or -verify and an export directory")
		return 2
	}
	if *check {
		report, err := audit.Verify(job.Dir(where))
		if err != nil {
			return fail(err)
		}
		for _, problem := range report.Problems {
			fmt.Println(problem)
		}
		for _, name := range report.Redacted {
			fmt.Printf("%s: redacted by the export's policy, so only its original can be matched to the run\n", name)
		}
		if len(report.Problems) > 0 {
			return 1
		}
		fmt.Printf("%d files are the run's own bytes, and audit.json says what they say\n", report.Covered)
		return 0
	}
	policy, err := audit.ReadPolicy(*policyFile)
	if err != nil {
		return fail(err)
	}
	if *to == "" {
		*to = filepath.Clean(where) + ".audit"
	}
	record, err := audit.Export(job.Dir(where), *to, audit.Options{Policy: policy, RequestLog: *requestLog, Now: evidence.Now})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("%s\n%d files carried, %d redacted\n", filepath.Join(*to, audit.SummaryFile), len(record.Carried), len(record.Redactions))
	return 0
}
