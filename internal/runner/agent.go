package runner

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/getlatentic/gusto/pkg/job"
)

// AgentUIDEnv names the user an agent runs as inside the box. The runner
// stays root there, so the record in out/ is written by the runner alone: the
// agent, and whatever code it runs, can read its workspace and its own home
// and cannot touch the record, the proxy's log included.
const AgentUIDEnv = "GUSTO_AGENT_UID"

// agent is that user. A runner that is not root, or an image that names no
// agent, has nobody to hand the work to and runs the harness as itself: a
// laptop, `gusto inside` by hand, GUSTO_SANDBOX=here.
type agent struct {
	uid, gid int
	present  bool
}

func agentHere() agent {
	uid, err := strconv.Atoi(os.Getenv(AgentUIDEnv))
	if err != nil || uid <= 0 || os.Geteuid() != 0 {
		return agent{}
	}
	return agent{uid: uid, gid: uid, present: true}
}

// handOver gives the agent its workspace and home, closes out/ to it, and
// sets the command to start as the agent.
func (a agent) handOver(dir job.Dir, command *exec.Cmd) error {
	if !a.present {
		return nil
	}
	for _, mine := range []string{dir.Workspace(), os.Getenv("HOME")} {
		if mine == "" {
			continue
		}
		if err := a.own(mine); err != nil {
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

// takeBack gives the workspace and home back to the runner's user when the harness is gone. A conversation's workspace is the
// instance's own folder, mounted from the machine the box runs on, where the runner's user is the server's and the agent's is a
// stranger: what the agent made would be files the server can list and cannot open, or remove. It is handed over again, whole, at the
// start of the next turn. It does what it can: a file it cannot change is left, and the run is not failed for it.
func (a agent) takeBack(dir job.Dir) {
	if !a.present {
		return
	}
	for _, mine := range []string{dir.Workspace(), os.Getenv("HOME")} {
		if mine != "" {
			_ = chownTree(mine, os.Geteuid(), os.Getegid())
		}
	}
}

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
