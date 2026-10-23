package main

import (
	"flag"
	"os"

	"github.com/getlatentic/gusto/internal/matrix"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/job"
)

type benchmarkFlags struct {
	into, again string
	here, json  bool
}

func declareBenchmark(set *flag.FlagSet) *benchmarkFlags {
	f := &benchmarkFlags{}
	set.StringVar(&f.into, "into", "", "where the benchmark goes, as a `directory`; beside the plan by default")
	set.BoolVar(&f.here, "here", false, "run here, without a container")
	set.StringVar(&f.again, "again", "", "run finished pairs again: `all`, or agent@model,...")
	set.BoolVar(&f.json, "json", false, "print the table as the catalog reads it")
	return f
}

// benchmarkTasks runs a plan's tasks on every agent and model pair it names, or
// prints a benchmark that already ran. In a container every run goes out the
// way a server's task does, through the proxy when a credential is held, so
// its cost is the proxy's count. It exits 0 when every run left a verdict
// and its record checks, whichever pairs finished, and 2 otherwise.
func benchmarkTasks(args []string) int {
	set := newSet("benchmark")
	f := declareBenchmark(set)
	named, code, ok := interspersed("benchmark", set, args)
	if !ok {
		return code
	}
	if len(named) != 1 {
		return usageError("benchmark", "needs a plan, or a benchmark that ran")
	}
	way, err := matrixWay(f.here)
	if err != nil {
		return fail(err)
	}
	exit, err := matrix.Answer(os.Stdout, named[0], matrix.Asked{
		Into: f.into, Again: f.again, JSON: f.json, Way: way, Progress: os.Stderr,
	})
	if err != nil {
		return fail(err)
	}
	return exit
}

// matrixWay is where a plan's runs go: in this process, in a sandbox on a
// gateway when the deployment runs tasks there, and in a container otherwise.
func matrixWay(here bool) (matrix.Way, error) {
	if here {
		return matrix.Here(), nil
	}
	if os.Getenv("GUSTO_SANDBOX") != "openshell" {
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
