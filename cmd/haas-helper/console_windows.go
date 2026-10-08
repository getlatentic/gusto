package main

import (
	"os"
	"syscall"
)

// The helper is built as a program with no window of its own, so that starting it at login does not open a black one. A program built
// that way has no console unless it is given one, so when it is started from a terminal it asks for that terminal's, and what it
// prints reaches the person who ran it. Started with its output sent somewhere, it keeps that.
func init() {
	attach := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	const attachParent = ^uintptr(0)
	if ok, _, _ := attach.Call(attachParent); ok == 0 {
		return
	}
	if !isOpen(os.Stdout) {
		if console, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stdout = console
		}
	}
	if !isOpen(os.Stderr) {
		if console, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stderr = console
		}
	}
	if !isOpen(os.Stdin) {
		if console, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
			os.Stdin = console
		}
	}
}

// isOpen says whether a standard stream was handed to the program: a program with no console is given none, and its streams are not
// usable until it has attached to one.
func isOpen(file *os.File) bool {
	handle := syscall.Handle(file.Fd())
	if handle == 0 || handle == syscall.InvalidHandle {
		return false
	}
	kernel := syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileType")
	kind, _, _ := kernel.Call(uintptr(handle))
	return kind != 0
}
