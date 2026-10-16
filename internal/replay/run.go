package replay

import (
	"flag"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// Here runs a replay in this process, the way `gusto inside` does.
func Here(dir job.Dir) error {
	_, err := runner.Inside(dir)
	return err
}

// Contained runs a replay in a container, the way `gusto run` does: the clone
// happens on this side of the box, so a repository credential never enters it.
// The exit code is the gate's, and the verdict is read from the evidence.
func Contained(dir job.Dir) error {
	spec, err := job.Read(dir)
	if err != nil {
		return err
	}
	if err := runner.Fill(spec, dir); err != nil {
		return err
	}
	_, err = sandbox.Run(dir, harness.ModelURL(), "")
	return err
}

// Where is Here when asked for, and a container otherwise.
func Where(here bool) Run {
	if here {
		return Here
	}
	return Contained
}

// Interspersed parses flags wherever they sit among the directories and returns
// the directories. Go's flag package stops at the first one, and would read
// `gusto replay a b -here` as three jobs, the last called -here.
func Interspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var named []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		if flags.NArg() == 0 {
			return named, nil
		}
		named = append(named, flags.Arg(0))
		args = flags.Args()[1:]
	}
}
