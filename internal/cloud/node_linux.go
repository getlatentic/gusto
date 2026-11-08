package cloud

import "syscall"

// dieWithParent has the kernel end a server when the process that started it does,
// however it ends.
func dieWithParent() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
