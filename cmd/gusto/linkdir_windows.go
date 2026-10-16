package main

import (
	"fmt"
	"os/exec"
)

// linkDir makes link a directory junction to target. A symbolic link on Windows needs a privilege a person does not have, and
// a junction, which works for a folder on the same computer, does not.
func linkDir(target, link string) error {
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J %s %s: %w: %s", link, target, err, out)
	}
	return nil
}
