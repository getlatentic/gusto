//go:build !linux

package cloud

import "syscall"

// dieWithParent is only the process group here: the kernel gives no way to have
// a child follow its parent, and a front door that is killed leaves its servers.
func dieWithParent() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
