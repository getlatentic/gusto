// Package service makes the helper start when its person signs in to their computer, and stop being started when they ask. It
// uses what each system already has for a program a user runs: a launch agent on macOS, a user unit of systemd on Linux and a
// scheduled task at logon on Windows. None of them needs an administrator, and none runs the helper as anybody but the person.
package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Name is what the service is called on every system.
const Name = "haas-helper"

// label is macOS's reverse-domain name for a launch agent.
const label = "com.latentic.haas-helper"

// Spec is what to start: the program, and the server it is to follow.
type Spec struct {
	// Program is the helper's own file, as an absolute path.
	Program string
	Server  string
	// Home is where the person's files are, for the log and the unit. Empty is the user's home.
	Home string
}

// Run executes a command and gives what it printed. A test replaces it.
type Run func(name string, args ...string) (string, error)

func execute(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Manager installs, removes and asks about the service on one system.
type Manager struct {
	GOOS string
	Run  Run
	// Uid is the person's user id, which launchd needs.
	Uid int
}

// Here is the manager for this computer.
func Here() Manager { return Manager{GOOS: runtime.GOOS, Run: execute, Uid: os.Getuid()} }

func (s Spec) home() (string, error) {
	if s.Home != "" {
		return s.Home, nil
	}
	return os.UserHomeDir()
}

// absolute says whether a path names a whole place on that system. The check is by the system the service is for, since a
// Windows path is not one on another system.
func absolute(goos, path string) bool {
	if goos == "windows" {
		return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') || strings.HasPrefix(path, `\\`)
	}
	return filepath.IsAbs(path)
}

func (s Spec) check(goos string) error {
	if !absolute(goos, s.Program) {
		return errors.New("the helper's program has to be named by its whole path")
	}
	if s.Server == "" {
		return errors.New("name the server the helper follows")
	}
	return nil
}

// Install makes the helper start at login, and starts it now.
func (m Manager) Install(spec Spec) (string, error) {
	if err := spec.check(m.GOOS); err != nil {
		return "", err
	}
	switch m.GOOS {
	case "darwin":
		return m.installLaunchd(spec)
	case "linux":
		return m.installSystemd(spec)
	case "windows":
		return m.installTask(spec)
	}
	return "", fmt.Errorf("a service is not made on %s", m.GOOS)
}

// Uninstall stops the helper and stops it starting at login. A service that was never installed is not an error.
func (m Manager) Uninstall(spec Spec) error {
	switch m.GOOS {
	case "darwin":
		return m.uninstallLaunchd(spec)
	case "linux":
		return m.uninstallSystemd(spec)
	case "windows":
		return m.uninstallTask()
	}
	return fmt.Errorf("a service is not made on %s", m.GOOS)
}

// Installed says whether the system has the service, and what it said.
func (m Manager) Installed(spec Spec) (bool, string) {
	switch m.GOOS {
	case "darwin":
		out, err := m.Run("launchctl", "print", fmt.Sprintf("gui/%d/%s", m.Uid, label))
		return err == nil, out
	case "linux":
		out, err := m.Run("systemctl", "--user", "is-enabled", Name+".service")
		return err == nil, out
	case "windows":
		out, err := m.Run("schtasks", "/Query", "/TN", Name)
		return err == nil, out
	}
	return false, ""
}
