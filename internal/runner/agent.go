package runner

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/getlatentic/gusto/pkg/job"
)

// agent is the user an agent runs as inside the box. The runner stays root
// there, so the record in out/ is written by the runner alone: the agent, and
// whatever code it runs, can read its workspace and its own home and cannot
// touch the record, the proxy's log included. A runner that is not root, or an
// image that names no agent, has nobody to hand the work to and runs the
// harness as itself: a laptop, `gusto inside` by hand, GUSTO_SANDBOX=here.
type agent struct {
	uid, gid int
	present  bool
	// keptHome is a home that is the person's own, kept between their tasks.
	keptHome bool
	// skills is where the harness finds its skills, which a kept home is given afresh for each task.
	skills string
}

func agentIn(place Place) agent {
	if place.AgentUID <= 0 || os.Geteuid() != 0 {
		return agent{}
	}
	return agent{uid: place.AgentUID, gid: place.AgentUID, present: true, keptHome: place.KeptHome}
}

// handOver gives the agent its workspace and home, closes out/ to it, and
// sets the command to start as the agent.
func (a agent) handOver(dir job.Dir, command *exec.Cmd) error {
	if !a.present {
		return nil
	}
	for _, mine := range append([]string{dir.Workspace()}, a.homeToGive()...) {
		if mine == "" {
			continue
		}
		if err := a.own(mine); err != nil {
			return err
		}
	}
	if a.keptHome {
		// The home's own folder, and not what is in it: everything else there the agent made, and is the agent's already.
		if err := os.Lchown(os.Getenv("HOME"), a.uid, a.gid); err != nil {
			return err
		}
	}
	if err := os.Chmod(dir.Out(), 0o700); err != nil {
		return err
	}
	a.runAs(command)
	return nil
}

func (a agent) own(root string) error { return chownTree(root, a.uid, a.gid) }

func chownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, uid, gid)
	})
}

// takeBack gives the task directory, the workspace and the home to whoever owns the task directory, once the record is written
// and the harness is gone. That is the server that made the directory, on the machine the box runs on. The runner's own user is
// not: under a container runtime whose root is the machine's root, what the box left would be files the server can list and
// cannot open or remove. The workspace is a conversation's own folder when it is mounted from elsewhere, and is handed over
// again, whole, at the start of the next turn. It does what it can: a file it cannot change is left, and the run is not failed.
func (a agent) takeBack(dir job.Dir) {
	if !a.present {
		return
	}
	uid, gid := os.Geteuid(), os.Getegid()
	if owner, owned := ownerOfPath(dir.Path()); owned {
		uid, gid = owner.uid, owner.gid
	}
	for _, mine := range append([]string{dir.Path(), dir.Workspace()}, a.homeToTakeBack()...) {
		if mine != "" {
			_ = chownTree(mine, uid, gid)
		}
	}
}

// owner is who a file belongs to.
type owner struct{ uid, gid int }

// endAll stops every process the agent left behind, so nothing it started is
// still running while the record is written.
func (a agent) endAll() {
	if !a.present {
		return
	}
	stop := exec.Command("sh", "-c", "kill -9 -1")
	a.runAs(stop)
	_ = stop.Run()
}
