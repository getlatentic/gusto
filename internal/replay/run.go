package replay

import (
	"context"
	"flag"

	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// Here runs a rerun in this process, the way `gusto inside` does, with its record signed as signs.
func Here(signs signing.Identity) Run {
	return func(dir job.Dir) error {
		_, err := runner.InsideContext(context.Background(), dir, runner.Place{Signing: signs})
		return err
	}
}

// Contained runs a rerun in a container, the way `gusto run` does: the clone
// happens on this side of the box, so a repository credential never enters it.
// The exit code is the check's, and the verdict is read from the evidence.
func Contained(box sandbox.Containers) Run {
	return func(dir job.Dir) error {
		spec, err := job.Read(dir)
		if err != nil {
			return err
		}
		if err := runner.Fill(spec, dir); err != nil {
			return err
		}
		_, err = box.Run(context.Background(), dir, harness.ModelURL(), "", sandbox.Owner{})
		return err
	}
}

// Where is Here when asked for, and a container otherwise.
func Where(here bool, box sandbox.Containers) Run {
	if here {
		return Here(box.Signing)
	}
	return Contained(box)
}

// Interspersed parses flags wherever they sit among the directories and returns
// the directories. Go's flag package stops at the first one, and would read
// `gusto rerun a b -here` as three tasks, the last called -here.
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
