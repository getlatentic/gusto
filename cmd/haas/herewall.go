package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/seatbelt"
)

// behindSeatbelt is what puts a job's engine, and the checks it runs, behind this Mac's own sandbox. The job can write its workspace, its
// own home, its conversation's state and its cache; it can read none of the person's home but those, its own folder, and where its
// engine and the programs on its PATH are; and it reaches nothing on the network but haas's proxy, unless the network is left open.
func behindSeatbelt(dir job.Dir, spec job.Spec, home, data, proxyPort string) (func(*exec.Cmd), error) {
	person, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	person, _ = filepath.EvalSymlinks(person)
	workspace, err := filepath.EvalSymlinks(dir.Workspace())
	if err != nil {
		return nil, err
	}
	policy := seatbelt.Policy{
		Home:     person,
		Writable: []string{workspace, home, data, sharedCache()},
		Readable: append([]string{mustAbs(dir.Path())}, programFolders(spec.Harness, person)...),
	}
	if os.Getenv("HAAS_JOB_NETWORK") != "open" {
		if policy.ProxyPort, err = strconv.Atoi(proxyPort); err != nil {
			return nil, err
		}
	}
	profile := filepath.Join(mustAbs(dir.Path()), "sandbox.sb")
	if err := os.WriteFile(profile, []byte(policy.Profile()), 0o600); err != nil {
		return nil, err
	}
	return seatbelt.Wrap(profile), nil
}

// programFolders are the folders of the person's home that the engine and the programs it starts are in: where its program really is,
// and every folder of the PATH that is under their home, such as the one node is in.
func programFolders(name, person string) []string {
	var folders []string
	if adapter, err := harness.Load("", name); err == nil {
		if found, err := exec.LookPath(adapter.Installed()); err == nil {
			folders = append(folders, seatbelt.Readable(found, person)...)
		}
	}
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if resolved, err := filepath.EvalSymlinks(entry); err == nil && strings.HasPrefix(resolved, person+string(filepath.Separator)) {
			folders = append(folders, resolved)
		}
	}
	return folders
}

func mustAbs(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return absolute
	}
	return resolved
}
