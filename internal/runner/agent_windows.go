package runner

import "os/exec"

// runAs does nothing here. The agent is a user of the Linux box a job runs in, and a job never runs in one on Windows.
func (a agent) runAs(command *exec.Cmd) {}
