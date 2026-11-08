//go:build !linux

package main

import "syscall"

// A sandbox is Linux; elsewhere the forwarder ends by its pipe alone, and its memory is the user's, which a test runs it under.

func endsWithParent() *syscall.SysProcAttr { return nil }

func hideMemory() {}
