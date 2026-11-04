//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

// ownGroup puts the harness in a process group of its own and has the whole group killed when the run's context ends. A
// harness is a program that starts others, and one that is killed alone leaves them holding its output open: the run would
// wait for them to finish, which is the end of the budget at best and never at worst.
func ownGroup(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		// A negative id is the group. A group that is already gone is not an error worth stopping a stop for.
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			return command.Process.Kill()
		}
		go killStragglers(-command.Process.Pid)
		return nil
	}
}

// killStragglers kills the group again until it is empty. A child that was being forked when the group was killed can miss the
// signal, and would hold the harness's output open until it ended. It stops after a second, so a group id used again later is
// left alone.
func killStragglers(group int) {
	for range 20 {
		time.Sleep(50 * time.Millisecond)
		if syscall.Kill(group, syscall.SIGKILL) != nil {
			return
		}
	}
}
