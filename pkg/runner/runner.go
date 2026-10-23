// Package runner is the part of gusto's runner that code outside this module may call: it
// runs an agent in a box (pkg/box) the caller provides and writes the signed record of the
// run. It forwards to the runner behind it and adds no behaviour of its own.
package runner

import (
	"context"

	internal "github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/box"
	"github.com/getlatentic/gusto/pkg/job"
)

// Outside does the whole run for an agent that runs in a box, and returns the exit
// code the check earned. The workspace of dir must already hold the repository.
func Outside(dir job.Dir, b box.Box) (int, error) { return internal.Outside(dir, b) }

// OutsideContext is Outside that stops early when its context ends.
func OutsideContext(ctx context.Context, dir job.Dir, b box.Box) (int, error) {
	return internal.OutsideContext(ctx, dir, b)
}

// Refuse keeps the reason a task never started in the task's transcript, where a person
// reading the task looks, and returns the reason.
func Refuse(dir job.Dir, reason error) error { return internal.Refuse(dir, reason) }
