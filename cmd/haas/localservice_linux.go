package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func userUnit() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "systemd", "user", "haas.service"), nil
}

// installService makes haas a systemd unit of this person's: started at login, started again if it fails, and started now.
func installService(home string) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	if program, err = filepath.EvalSymlinks(program); err != nil {
		return err
	}
	unit, err := userUnit()
	if err != nil {
		return err
	}
	text := fmt.Sprintf(`[Unit]
Description=haas, the agents on this computer

[Service]
ExecStart=%q local run
Restart=on-failure

[Install]
WantedBy=default.target
`, program)
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unit, []byte(text), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", "haas.service"}, {"--user", "restart", "haas.service"}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %v: %w: %s; without a systemd user session, run haas local run yourself", args[1:], err, out)
		}
	}
	return nil
}

func stopService() error {
	if out, err := exec.Command("systemctl", "--user", "stop", "haas.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop: %w: %s", err, out)
	}
	return nil
}

func uninstallService() error {
	exec.Command("systemctl", "--user", "disable", "--now", "haas.service").Run()
	unit, err := userUnit()
	if err != nil {
		return err
	}
	if err := os.Remove(unit); err != nil && !os.IsNotExist(err) {
		return err
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
