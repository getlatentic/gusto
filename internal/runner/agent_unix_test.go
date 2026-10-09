//go:build !windows

package runner

import (
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/getlatentic/haas/internal/job"
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
