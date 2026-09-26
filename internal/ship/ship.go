// Package ship hands a finished job back as a pull request.
//
// A job whose gate passed has a patch and a manifest that hashes it. Shipping
// is putting that patch on a branch at the commit the job ran against, pushing
// the branch, and asking the forge for a pull request that says where the
// patch came from. Review, and everything after it, belongs to a person: the
// forge here can open a pull request and do nothing else.
package ship

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ErrRefused marks a job that must not ship. The caller tells it apart from a
// failure to ship, because a refusal is the answer and not a fault.
var ErrRefused = errors.New("refused")

// Pusher puts the plan's branch on its remote.
type Pusher interface {
	Push(ctx context.Context, plan Plan) error
}

// Forge opens a pull request and returns its address. It has one method on
// purpose: nothing in this package can do anything to a pull request once it
// is open.
type Forge interface {
	Open(ctx context.Context, pull Pull) (string, error)
}

// Pull is what a forge needs to open a pull request.
type Pull struct {
	Owner string
	Name  string
	Head  string
	Base  string
	Title string
	Body  string
}

// Shipper says what it is about to do, and then does it unless it is a dry run.
type Shipper struct {
	Pusher Pusher
	Forge  Forge
	Out    io.Writer
	DryRun bool
}

// Ship pushes the branch and opens the pull request, in that order, so a pull
// request never names a branch that is not there. A dry run prints the plan and
// stops before either.
func (s Shipper) Ship(ctx context.Context, plan Plan) (string, error) {
	if _, err := io.WriteString(s.Out, plan.Describe(s.DryRun)); err != nil {
		return "", err
	}
	if s.DryRun {
		return "", nil
	}
	if err := s.Pusher.Push(ctx, plan); err != nil {
		return "", fmt.Errorf("pushing %s: %w", plan.Branch, err)
	}
	address, err := s.Forge.Open(ctx, plan.Pull())
	if err != nil {
		return "", fmt.Errorf("the branch %s is pushed, and opening its pull request failed: %w", plan.Branch, err)
	}
	return address, nil
}
