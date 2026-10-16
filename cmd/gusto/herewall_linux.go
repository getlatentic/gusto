package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/landlock"
)

// wrapProcess holds a job to the policy with Linux's Landlock, applied by gusto itself just before it starts the engine. A Linux
// computer keeps its programs outside the home too, so every folder of the PATH is readable. The policy is written beside the job.
func wrapProcess(dir job.Dir, p wallPolicy) (func(*exec.Cmd), error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Landlock allows a folder that exists, and the cache is made by the first job that wants it.
	for _, folder := range p.Writable {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			return nil, err
		}
	}
	policy := landlock.Policy{Read: append([]string{}, p.Readable...), Write: p.Writable, ProxyPort: p.ProxyPort}
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if resolved, err := filepath.EvalSymlinks(entry); err == nil {
			policy.Read = append(policy.Read, resolved)
		}
	}
	file := filepath.Join(mustAbs(dir.Path()), "landlock.json")
	if err := policy.Save(file); err != nil {
		return nil, err
	}
	return landlock.Wrap(self, file), nil
}
