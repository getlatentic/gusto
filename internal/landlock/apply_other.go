//go:build !linux

package landlock

import "errors"

var errNotLinux = errors.New("Landlock is Linux's")

// ABI says Landlock is not here.
func ABI() (int, error) { return 0, errNotLinux }

// Apply refuses: there is nothing to restrict with.
func Apply(Policy) error { return errNotLinux }
