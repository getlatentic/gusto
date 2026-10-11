package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
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
	me, err := user.Current()
	if err != nil {
		return err
	}
	definition := filepath.Join(home, "logon-task.xml")
	if err := os.WriteFile(definition, logonTask(me.Username, program), 0o600); err != nil {
		return err
	}
	defer os.Remove(definition)
	if out, err := exec.Command("schtasks", "/Create", "/F", "/TN", taskName, "/XML", definition).CombinedOutput(); err != nil {
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
