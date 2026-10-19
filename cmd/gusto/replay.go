package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/replay"
)

// replayJobs runs finished jobs again at the commit each started from and
// prints one summary. It exits 1 when any verdict got worse, 2 when a job could
// not be replayed, and 0 when every verdict held or improved.
func replayJobs(args []string) int {
	flags := flag.NewFlagSet("replay", flag.ContinueOnError)
	into := flags.String("into", "", "where the replays go, beside each job by default")
	here := flags.Bool("here", false, "run here, without a container")
	named, err := replay.Interspersed(flags, args)
	if err != nil {
		return 2
	}
	if len(named) == 0 {
		fmt.Fprintln(os.Stderr, "gusto replay needs a job directory, or a directory of jobs")
		return 2
	}
	olds, err := replay.Expand(named)
	if err != nil {
		return fail(err)
	}
	rows := replay.Sweep(olds, *into, replay.Where(*here))
	replay.Print(os.Stdout, rows)
	return replay.ExitCode(rows)
}
