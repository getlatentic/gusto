//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
)

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

// ownerOfPath is who a file belongs to, and false when that cannot be read.
func ownerOfPath(path string) (owner, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return owner{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return owner{}, false
	}
	return owner{uid: int(stat.Uid), gid: int(stat.Gid)}, true
}
