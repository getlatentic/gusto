package main

import (
	"flag"
	"os"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/replay"
)

// compareJobs runs one job on several harnesses as one comparison, or prints a
// comparison that already ran. It exits 0 when every side ran and its evidence
// checks, whichever harness did better, and 2 otherwise.
func compareJobs(args []string) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	into := flags.String("into", "", "where the comparison goes, beside the job by default")
	here := flags.Bool("here", false, "run here, without a container")
	named, err := replay.Interspersed(flags, args)
	if err != nil {
		return 2
	}
	exit, err := compare.Answer(os.Stdout, named, *into, replay.Where(*here))
	if err != nil {
		return fail(err)
	}
	return exit
}
