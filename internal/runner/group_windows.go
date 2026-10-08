package runner

import "os/exec"

// ownGroup does nothing on Windows, where a job's harness does not run: the run is in a Linux box.
func ownGroup(command *exec.Cmd) {}
