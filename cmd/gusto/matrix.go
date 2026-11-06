package main

import (
	"context"
	"flag"
	"os"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/matrix"
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

// matrixWay is where a plan's runs go: in this process, or in the box a server's turns run in, through the proxy when a
// credential is held.
func matrixWay(here bool) (matrix.Way, error) {
	cfg, _, err := loadServe(os.Getenv)
	if err != nil {
		return matrix.Way{}, err
	}
	signs, err := cfg.sign.identity()
	if err != nil {
		return matrix.Way{}, err
	}
	if here {
		return matrix.Here(signs), nil
	}
	boxes, err := openBackend(cfg, signs, nil, "")
	if err != nil {
		return matrix.Way{}, err
	}
	return matrix.Contained(boxes.String(), func(root string) (matrix.Door, error) {
		way, err := openWayOut(root, cfg.way, cfg.core.local)
		if err != nil {
			return nil, err
		}
		return benchmarkDoor{way, boxes}, nil
	}), nil
}

// benchmarkDoor is a way out whose runs start in the server's boxes.
type benchmarkDoor struct {
	way   *WayOut
	boxes backend.Backend
}

func (d benchmarkDoor) Listen() (func(), error) { return d.way.Listen(d.boxes.Reach()) }

func (d benchmarkDoor) Run(dir job.Dir, spec job.Spec) (int, error) {
	return d.boxes.Run(context.Background(), backend.Turn{Dir: dir, Spec: spec, Exit: d.way.exit()})
}

func (d benchmarkDoor) Done(dir job.Dir) { d.way.Done(dir) }
