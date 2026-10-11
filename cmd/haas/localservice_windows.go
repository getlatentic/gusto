package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// taskName names haas to the Task Scheduler.
const taskName = "Latentic haas"

// installService makes haas a task that starts when this person logs on, and starts it now.
func installService(home string) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	if program, err = filepath.EvalSymlinks(program); err != nil {
		return err
	}
	command := fmt.Sprintf(`"%s" local run`, program)
	if out, err := exec.Command("schtasks", "/Create", "/F", "/SC", "ONLOGON", "/TN", taskName, "/TR", command).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Create: %w: %s", err, out)
	}
	exec.Command("schtasks", "/End", "/TN", taskName).Run()
	if out, err := exec.Command("schtasks", "/Run", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Run: %w: %s", err, out)
	}
	return nil
}

func stopService() error {
	if out, err := exec.Command("schtasks", "/End", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /End: %w: %s", err, out)
	}
	return nil
}

func uninstallService() error {
	exec.Command("schtasks", "/End", "/TN", taskName).Run()
	if out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Delete: %w: %s", err, out)
	}
	return nil
}
