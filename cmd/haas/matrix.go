package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/matrix"
	"github.com/getlatentic/haas/internal/replay"
)

// matrixJobs runs a plan's jobs on every harness and model pair it names, or
// prints a matrix that already ran. In a container every cell goes out the
// way a server's job does, through the proxy when a credential is held, so
// its cost is the proxy's count. It exits 0 when every cell left a verdict
// and its evidence checks, whichever pairs finished, and 2 otherwise.
func matrixJobs(args []string) int {
	flags := flag.NewFlagSet("matrix", flag.ContinueOnError)
	into := flags.String("into", "", "where the matrix goes, beside the plan by default")
	here := flags.Bool("here", false, "run here, without a container")
	again := flags.String("again", "", `run finished pairs again: "all", or harness@model,...`)
	asJSON := flags.Bool("json", false, "print the table as the catalog reads it")
	named, err := replay.Interspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(named) != 1 {
		fmt.Fprintln(os.Stderr, "haas matrix needs a plan, or a matrix that ran")
		return 2
	}
	way := matrix.Contained(func(root string) (matrix.Door, error) { return openWayOut(root) })
	if *here {
		way = matrix.Here()
	}
	exit, err := matrix.Answer(os.Stdout, named[0], matrix.Asked{
		Into: *into, Again: *again, JSON: *asJSON, Way: way, Progress: os.Stderr,
	})
	if err != nil {
		return fail(err)
	}
	return exit
}
