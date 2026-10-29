//go:build !windows

package runner

import (
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

func ownerOf(info fs.FileInfo) uint32 { return info.Sys().(*syscall.Stat_t).Uid }

// What the harness made is the runner's again once the harness is gone, so whoever owns the folder outside the box can read and remove it.
func TestWhatTheAgentMadeIsTheRunnersAgain(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("handing a file to another user takes root")
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	file := dir.Workspace() + "/made-by-the-harness"
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent := agent{uid: 1000, gid: 1000, present: true}
	if err := agent.own(dir.Workspace()); err != nil {
		t.Fatal(err)
	}
	agent.takeBack(dir)
	info, _ := os.Stat(file)
	if owner := ownerOf(info); owner != uint32(os.Geteuid()) {
		t.Errorf("the file is owned by %d, not by the runner", owner)
	}
}

// Under a container runtime whose root is the machine's, the task directory belongs to the server that made it, and so does
// everything the box leaves in it once the run is over: the workspace, and the record the runner wrote as root.
func TestWhatTheBoxLeftBelongsToWhoeverOwnsTheTaskDirectory(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("handing a file to another user takes root")
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	const server = 4321
	if err := os.Chown(dir.Path(), server, server); err != nil {
		t.Fatal(err)
	}
	made := []string{dir.Workspace() + "/made-by-the-harness", dir.OutFile("manifest.json")}
	for _, file := range made {
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	agent := agent{uid: 1000, gid: 1000, present: true}
	if err := agent.own(dir.Workspace()); err != nil {
		t.Fatal(err)
	}
	agent.takeBack(dir)
	for _, file := range append(made, dir.Out(), dir.Workspace()) {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if owner := ownerOf(info); owner != server {
			t.Errorf("%s is owned by %d, not by the task directory's owner", file, owner)
		}
	}
}
