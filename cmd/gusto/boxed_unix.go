//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// inheritable lets a file stay open in the program this one becomes.
func inheritable(file *os.File) error {
	_, err := unix.FcntlInt(file.Fd(), unix.F_SETFD, 0)
	return err
}
