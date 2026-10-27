package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// endsWithParent has the forwarder killed when the thread that started it, which becomes the agent, ends.
func endsWithParent() *syscall.SysProcAttr { return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL} }

// hideMemory keeps another process of the same user, the agent among them, from reading the forwarder's memory, where its key is.
func hideMemory() { _ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) }
