package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/seatbelt"
)

// wallPolicy is what a task run here may do, whichever sandbox this computer has to hold it to that.
type wallPolicy struct {
	// Home is the person's home, which is closed to reading but for Readable.
	Home string
	// Writable are the folders the task may write, and Readable the ones under the home it may still read.
	Writable, Readable []string
	// ProxyPort is the one port on this computer the task may connect to, or zero when the network is left open.
	ProxyPort int
}

// behindProcessWall is what puts a task's engine, and the checks it runs, behind this computer's own sandbox. The task can write its
// workspace, its own home, its conversation's state and its cache; it can read none of the person's home but those, its own folder,
// and where its engine and the programs on its PATH are; and it reaches nothing on the network but gusto's proxy, unless the network
// is left open.
func behindProcessWall(dir job.Dir, spec job.Spec, home, data, proxyPort string) (func(*exec.Cmd), error) {
	person, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	person, _ = filepath.EvalSymlinks(person)
	workspace, err := filepath.EvalSymlinks(dir.Workspace())
	if err != nil {
		return nil, err
	}
	policy := wallPolicy{
		Home:     person,
		Writable: []string{workspace, home, data, sharedCache()},
		Readable: append([]string{mustAbs(dir.Path())}, programFolders(spec.Harness, person)...),
	}
	if os.Getenv("GUSTO_JOB_NETWORK") != "open" {
		if policy.ProxyPort, err = strconv.Atoi(proxyPort); err != nil {
			return nil, err
		}
	}
	return wrapProcess(dir, policy)
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
