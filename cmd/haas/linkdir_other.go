//go:build !windows

package main

import "os"

// linkDir makes link a symbolic link to the folder target.
func linkDir(target, link string) error { return os.Symlink(target, link) }
