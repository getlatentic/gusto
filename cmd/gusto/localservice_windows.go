package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// taskName names gusto to the Task Scheduler.
const taskName = "Latentic gusto"

// installService makes gusto a task that starts when this person logs on, and starts it now.
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
	// A gusto started before, by an earlier install, holds the port the one started now listens on.
	if err := endRunning(); err != nil {
		return err
	}
	if out, err := exec.Command("schtasks", "/Run", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Run: %w: %s", err, out)
	}
	return nil
}

func stopService() error {
	if out, err := exec.Command("schtasks", "/End", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /End: %w: %s", err, out)
	}
	return endRunning()
}

func uninstallService() error {
	exec.Command("schtasks", "/End", "/TN", taskName).Run()
	if out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks /Delete: %w: %s", err, out)
	}
	return endRunning()
}

// endRunning ends the gusto the task started. Ending the task ends the console it runs in and not gusto, so gusto says which
// process it is in gusto.pid.
func endRunning() error {
	home, err := localHome()
	if err != nil {
		return err
	}
	text, err := os.ReadFile(filepath.Join(home, "gusto.pid"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil || pid == os.Getpid() {
		return nil
	}
	if out, err := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid), "/FI", "IMAGENAME eq gusto.exe").CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %w: %s", err, out)
	}
	return os.Remove(filepath.Join(home, "gusto.pid"))
}

// keepLog writes gusto's log to gusto.log in its folder, since the console the task gives it is one nobody sees.
func keepLog(home string) error {
	file, err := os.OpenFile(filepath.Join(home, "gusto.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	os.Stdout, os.Stderr = file, file
	log.SetOutput(file)
	return nil
}
