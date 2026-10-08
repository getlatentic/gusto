package service

import (
	"fmt"
	"strings"
)

// TaskCommand is what the task runs: the program and its arguments, each in quotes, as one string Windows' scheduler takes.
func TaskCommand(spec Spec) string {
	quote := func(word string) string { return `"` + strings.ReplaceAll(word, `"`, `\"`) + `"` }
	return quote(spec.Program) + " run -server " + quote(spec.Server)
}

func (m Manager) installTask(spec Spec) (string, error) {
	// /F replaces a task of the same name, so a second install is the first made again. /RL LIMITED runs it as the person
	// with no more rights than they have.
	create := []string{"/Create", "/F", "/SC", "ONLOGON", "/RL", "LIMITED", "/TN", Name, "/TR", TaskCommand(spec)}
	if out, err := m.Run("schtasks", create...); err != nil {
		return "", fmt.Errorf("schtasks /Create: %v: %s", err, out)
	}
	if out, err := m.Run("schtasks", "/Run", "/TN", Name); err != nil {
		return "", fmt.Errorf("schtasks /Run: %v: %s", err, out)
	}
	return "started now and at every logon (a scheduled task called " + Name + ")", nil
}

func (m Manager) uninstallTask() error {
	if _, err := m.Run("schtasks", "/Query", "/TN", Name); err != nil {
		return nil
	}
	_, _ = m.Run("schtasks", "/End", "/TN", Name)
	if out, err := m.Run("schtasks", "/Delete", "/F", "/TN", Name); err != nil {
		return fmt.Errorf("schtasks /Delete: %v: %s", err, out)
	}
	return nil
}
