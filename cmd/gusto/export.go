package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/audit"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

type exportFlags struct {
	to, policy, requests string
	verify               bool
}

func declareExport(set *flag.FlagSet) *exportFlags {
	f := &exportFlags{}
	set.StringVar(&f.to, "to", "", "the new `directory` to write the export into (default <dir>.audit)")
	set.StringVar(&f.policy, "policy", "", "a JSON `file` saying what the export drops and redacts")
	set.StringVar(&f.requests, "requests", "", "the server's request log, one JSON line a request")
	set.BoolVar(&f.verify, "verify", false, "check an export instead of writing one")
	return f
}

// exportTask writes a task's audit export, or with -verify checks one. It is
// the command's only identifier in this package; everything it does is in
// internal/audit.
func exportTask(args []string) int {
	set := newSet("export")
	f := declareExport(set)
	where, rest := split(args)
	if code, ok := parseFlags("export", set, rest); !ok {
		return code
	}
	if where == "" && set.NArg() == 1 {
		where = set.Arg(0)
	}
	if where == "" {
		return usageError("export", "needs a task directory, or -verify and an export directory")
	}
	// The keyring names the signer both ways: the one an export is written
	// under, and the one a verifier checks that name against.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	if f.verify {
		report, err := audit.Verify(job.Dir(where), keyring)
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
	policy, err := audit.ReadPolicy(f.policy)
	if err != nil {
		return fail(err)
	}
	if f.to == "" {
		f.to = filepath.Clean(where) + ".audit"
	}
	record, err := audit.Export(job.Dir(where), f.to, audit.Options{Policy: policy, RequestLog: f.requests, Now: evidence.Now, Keyring: keyring})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("%s\n%d files carried, %d redacted\n", filepath.Join(f.to, audit.SummaryFile), len(record.Carried), len(record.Redactions))
	return 0
}
