// Package runner is the part of gusto's runner that code outside this module may call: it
// runs an agent in a box (pkg/box) the caller provides and writes the signed record of the
// run. It opens the key it is given and forwards to the runner behind it.
package runner

import (
	"context"

	internal "github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/box"
	"github.com/getlatentic/gusto/pkg/task"
)

// Signing is who stands behind a record: Key is the private key it is signed with, a PEM file
// from gusto keygen or awskms://<key>, and Name is what the record calls its signer, which is
// the key's id when it is empty. The zero Signing leaves a record unsigned.
type Signing struct {
	Key, Name string
}

// Outside does the whole run for an agent that runs in a box, and returns the exit code the
// check earned. The working folder of dir must already hold the repository. The run stops early
// when ctx ends, and a key that cannot be read stops it before the agent starts.
func Outside(ctx context.Context, dir task.Dir, b box.Box, signs Signing) (int, error) {
	identity, err := signing.Open(ctx, signs.Key, signs.Name)
	if err != nil {
		return 0, err
	}
	return internal.Outside(ctx, dir, b, identity)
}

// Refuse keeps the reason a task never started in the task's transcript, where a person
// reading the task looks, and returns the reason.
func Refuse(dir task.Dir, reason error) error { return internal.Refuse(dir, reason) }
