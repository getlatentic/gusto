//go:build !windows

package runner

import (
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
