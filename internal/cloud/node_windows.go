package cloud

import "syscall"

// dieWithParent is nothing here: a server of a gusto cloud server runs on Linux, and this only lets the program build.
func dieWithParent() *syscall.SysProcAttr { return &syscall.SysProcAttr{} }
