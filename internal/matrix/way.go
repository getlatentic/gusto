package matrix

import (
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/replay"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// Way is how the matrix starts a run, and what it writes down about where.
type Way struct {
	// Where is how the runs start, as the environment column says it.
	Where string
	// Open readies the runs of the comparison at root, and returns how to run
	// one and how to stop once they are done.
	Open func(root string) (replay.Run, func(), error)
}

// Here runs every cell in this process, the way `gusto inside` does. A harness
// run here reads this machine's own configuration, which is what makes a
// laptop's row differ from a container's.
func Here() Way {
	return Way{
		Where: "here, in this process",
		Open: func(string) (replay.Run, func(), error) {
			return replay.Here, func() {}, nil
		},
	}
}

// Door is the way out of a container the server uses: the proxy when a
// credential is held, which writes every call into the job's out/proxy.jsonl,
// and the model's own address when none is.
type Door interface {
	Listen() (func(), error)
	Run(dir job.Dir, spec job.Spec, mounts ...sandbox.Mount) (int, error)
	Done(dir job.Dir)
}

// Contained runs every cell through a door opened for each comparison, in the
// box that where says. The proxy names a job by its directory under the root it
// was opened on, so a comparison's runs are that root's children.
func Contained(where string, open func(root string) (Door, error)) Way {
	return Way{
		Where: where,
		Open: func(root string) (replay.Run, func(), error) {
			door, err := open(root)
			if err != nil {
				return nil, nil, err
			}
			stop, err := door.Listen()
			if err != nil {
				return nil, nil, err
			}
			return through(door), stop, nil
		},
	}
}

// through runs one cell the way the server runs a job: the checkout happens on
// this side of the box, so a repository credential never enters it, and the
// token is dropped when the run ends. The verdict is read from the evidence,
// not from the exit code.
func through(door Door) replay.Run {
	return func(dir job.Dir) error {
		spec, err := job.Read(dir)
		if err != nil {
			return err
		}
		if err := runner.Fill(spec, dir); err != nil {
			return err
		}
		defer door.Done(dir)
		_, err = door.Run(dir, spec)
		return err
	}
}
