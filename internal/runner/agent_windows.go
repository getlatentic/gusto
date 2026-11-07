package runner

import "os/exec"

// runAs does nothing here. The agent is a user of the Linux box a task runs in, and a task never runs in one on Windows.
func (a agent) runAs(command *exec.Cmd) {}

// ownerOfPath says nothing here, for the same reason.
func ownerOfPath(string) (owner, bool) { return owner{}, false }
