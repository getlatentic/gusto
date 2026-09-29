package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/matrix"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/replay"
	"github.com/getlatentic/haas/internal/sandbox"
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
	way, err := matrixWay(*here)
	if err != nil {
		return fail(err)
	}
	exit, err := matrix.Answer(os.Stdout, named[0], matrix.Asked{
		Into: *into, Again: *again, JSON: *asJSON, Way: way, Progress: os.Stderr,
	})
	if err != nil {
		return fail(err)
	}
	return exit
}

// matrixWay is where a plan's cells run: in this process, in a sandbox on a
// gateway when the deployment runs jobs there, and in a container otherwise.
func matrixWay(here bool) (matrix.Way, error) {
	if here {
		return matrix.Here(), nil
	}
	if os.Getenv("HAAS_SANDBOX") != "openshell" {
		where := "in a container, " + sandbox.Runtime() + " " + sandbox.Image()
		return matrix.Contained(where, func(root string) (matrix.Door, error) { return openWayOut(root) }), nil
	}
	cfg, err := openshell.FromEnv()
	if err != nil {
		return matrix.Way{}, err
	}
	where := "in an OpenShell sandbox, " + cfg.Image
	return matrix.Contained(where, func(root string) (matrix.Door, error) {
		way, err := openWayOut(root)
		if err != nil {
			return nil, err
		}
		return sandboxed{way, cfg}, nil
	}), nil
}

// sandboxed is a way out whose runs start in a sandbox on the gateway.
type sandboxed struct {
	*WayOut
	cfg openshell.Config
}

func (s sandboxed) Run(dir job.Dir, spec job.Spec, _ ...sandbox.Mount) (int, error) {
	return s.RunInSandbox(dir, spec, s.cfg)
}
