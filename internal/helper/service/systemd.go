package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s Spec) unitPath(home string) string {
	return filepath.Join(home, ".config", "systemd", "user", Name+".service")
}

// quoted is a word for systemd's command line, in double quotes with its own escapes.
func quoted(word string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`).Replace(word) + `"`
}

// Unit is the user unit: started when the person's session starts, and again a few seconds after it ends by itself.
func Unit(spec Spec) string {
	return fmt.Sprintf(`[Unit]
Description=Your computer's way to your organisation's agents

[Service]
ExecStart=%s run -server %s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, quoted(spec.Program), quoted(spec.Server))
}

func (m Manager) installSystemd(spec Spec) (string, error) {
	home, err := spec.home()
	if err != nil {
		return "", err
	}
	path := spec.unitPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(Unit(spec)), 0o644); err != nil {
		return "", err
	}
	for _, step := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "--now", Name + ".service"}} {
		if out, err := m.Run("systemctl", step...); err != nil {
			return "", fmt.Errorf("systemctl %s: %v: %s", strings.Join(step, " "), err, out)
		}
	}
	return "started now and at every login (" + path + "). To keep it running when you are not logged in, run `loginctl enable-linger`", nil
}

func (m Manager) uninstallSystemd(spec Spec) error {
	home, err := spec.home()
	if err != nil {
		return err
	}
	_, _ = m.Run("systemctl", "--user", "disable", "--now", Name+".service")
	if err := os.Remove(spec.unitPath(home)); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = m.Run("systemctl", "--user", "daemon-reload")
	return nil
}
