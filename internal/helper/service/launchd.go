package service

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
)

func (s Spec) plistPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// Plist is the launch agent: started at login, started again when it ends, and its words kept in a log the person can read.
func Plist(spec Spec, home string) string {
	log := filepath.Join(home, "Library", "Logs", Name+".log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
		<string>-server</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>ProcessType</key><string>Background</string>
	<key>StandardOutPath</key><string>%s</string>
	<key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, html.EscapeString(spec.Program), html.EscapeString(spec.Server), html.EscapeString(log), html.EscapeString(log))
}

func (m Manager) installLaunchd(spec Spec) (string, error) {
	home, err := spec.home()
	if err != nil {
		return "", err
	}
	path := spec.plistPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(home, "Library", "Logs"), 0o755); err != nil {
		return "", err
	}
	// A second install replaces the first: the old one is taken out before the new one goes in.
	_, _ = m.Run("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", m.Uid, label))
	if err := os.WriteFile(path, []byte(Plist(spec, home)), 0o644); err != nil {
		return "", err
	}
	if out, err := m.Run("launchctl", "bootstrap", fmt.Sprintf("gui/%d", m.Uid), path); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	return "started now and at every login (" + path + ")", nil
}

func (m Manager) uninstallLaunchd(spec Spec) error {
	home, err := spec.home()
	if err != nil {
		return err
	}
	_, _ = m.Run("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", m.Uid, label))
	if err := os.Remove(spec.plistPath(home)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
