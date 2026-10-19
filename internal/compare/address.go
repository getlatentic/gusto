package compare

import (
	"errors"
	"io"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/replay"
)

// Answer is the whole of `gusto compare`. A comparison's directory alone prints
// it again from the record; a submission and agents make a new one, run
// it, and print it. Either way the exit code is ExitCode's.
func Answer(w io.Writer, named []string, parent string, run replay.Run) (int, error) {
	where, sides, err := addressed(named, parent, run)
	if err != nil {
		return 0, err
	}
	record, err := Read(where)
	if err != nil {
		return 0, err
	}
	Print(w, where, record, sides)
	return ExitCode(sides), nil
}

// addressed is the comparison named and its sides, made and run first when
// what was named is a submission rather than a comparison. A run that fails is
// not an error here: its side says so, beside the sides that did run.
func addressed(named []string, parent string, run replay.Run) (string, []Side, error) {
	if len(named) == 0 {
		return "", nil, errors.New("gusto compare needs a task and the agents to run it on, or a comparison")
	}
	if len(named) == 1 && Is(named[0]) {
		record, err := Read(named[0])
		return named[0], Sides(named[0], record), err
	}
	where, err := Prepare(job.Dir(named[0]), parent, named[1:])
	if err != nil {
		return "", nil, err
	}
	sides, err := Start(where, run)
	return where, sides, err
}
