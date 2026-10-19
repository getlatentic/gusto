package main

import (
	"flag"
	"os"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/replay"
)

type compareFlags struct {
	into string
	here bool
}

func declareCompare(set *flag.FlagSet) *compareFlags {
	f := &compareFlags{}
	set.StringVar(&f.into, "into", "", "where the comparison goes, as a `directory`; beside the task by default")
	set.BoolVar(&f.here, "here", false, "run here, without a container")
	return f
}

// compareTasks runs one task on several agents as one comparison, or prints a
// comparison that already ran. It exits 0 when every side ran and its record
// checks, whichever agent did better, and 2 otherwise.
func compareTasks(args []string) int {
	set := newSet("compare")
	f := declareCompare(set)
	named, code, ok := interspersed("compare", set, args)
	if !ok {
		return code
	}
	if len(named) == 0 || len(named) == 2 {
		return usageError("compare", "needs a task and the agents to run it on, or a comparison")
	}
	exit, err := compare.Answer(os.Stdout, named, f.into, replay.Where(f.here))
	if err != nil {
		return fail(err)
	}
	return exit
}
