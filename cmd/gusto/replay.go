package main

import (
	"flag"
	"os"

	"github.com/getlatentic/gusto/internal/replay"
)

type rerunFlags struct {
	into string
	here bool
}

func declareRerun(set *flag.FlagSet) *rerunFlags {
	f := &rerunFlags{}
	set.StringVar(&f.into, "into", "", "where the reruns go, as a `directory`; beside each task by default")
	set.BoolVar(&f.here, "here", false, "run here, without a container")
	return f
}

// rerunTasks runs finished tasks again at the commit each started from and
// prints one summary. It exits 1 when any verdict got worse, 2 when a task could
// not be rerun, and 0 when every verdict held or improved.
func rerunTasks(args []string) int {
	set := newSet("rerun")
	f := declareRerun(set)
	named, code, ok := interspersed("rerun", set, args)
	if !ok {
		return code
	}
	if len(named) == 0 {
		return usageError("rerun", "needs a task directory, or a directory of tasks")
	}
	olds, err := replay.Expand(named)
	if err != nil {
		return fail(err)
	}
	rows := replay.Sweep(olds, f.into, replay.Where(f.here))
	replay.Print(os.Stdout, rows)
	return replay.ExitCode(rows)
}
