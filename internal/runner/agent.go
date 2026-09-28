package runner

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/getlatentic/haas/internal/job"
)

// AgentUIDEnv names the user a harness runs as inside the box. The runner
// stays root there, so the record in out/ is written by the runner alone: the
// harness, and whatever code it runs, can read its workspace and its own home
// and cannot touch the record, the proxy's log included.
const AgentUIDEnv = "HAAS_AGENT_UID"

// agent is that user. A runner that is not root, or an image that names no
// agent, has nobody to hand the work to and runs the harness as itself: a
// laptop, `haas inside` by hand, HAAS_SANDBOX=here.
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

// trustWorkspace lets the runner's own git read a workspace the agent owns,
// which git otherwise refuses as another user's repository.
func (a agent) trustWorkspace() {
	if !a.present {
		return
	}
	os.Setenv("GIT_CONFIG_COUNT", "1")
	os.Setenv("GIT_CONFIG_KEY_0", "safe.directory")
	os.Setenv("GIT_CONFIG_VALUE_0", "*")
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

func (a agent) own(root string) error {
	return filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(path, a.uid, a.gid)
	})
}

// runAs starts the command as the agent, in a process group of its own.
func (a agent) runAs(command *exec.Cmd) {
	if !a.present {
		return
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(a.uid), Gid: uint32(a.gid)}
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
