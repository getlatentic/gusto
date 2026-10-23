package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/seatbelt"
	"github.com/getlatentic/gusto/pkg/job"
)

// wrapProcess holds a task to the policy with this Mac's Seatbelt. The profile is written beside the task.
func wrapProcess(dir job.Dir, p wallPolicy) (func(*exec.Cmd), error) {
	profile := filepath.Join(mustAbs(dir.Path()), "sandbox.sb")
	policy := seatbelt.Policy{Home: p.Home, Writable: p.Writable, Readable: p.Readable, ProxyPort: p.ProxyPort}
	if err := os.WriteFile(profile, []byte(policy.Profile()), 0o600); err != nil {
		return nil, err
	}
	return seatbelt.Wrap(profile), nil
}
