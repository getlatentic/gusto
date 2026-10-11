package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// serviceLabel names haas to launchd.
const serviceLabel = "com.latentic.haas"

func agentPlist() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist"), nil
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// installService makes haas a launch agent of this person's: started at login, started again if it stops, and started now.
func installService(home string) error {
	program, err := os.Executable()
	if err != nil {
		return err
	}
	if program, err = filepath.EvalSymlinks(program); err != nil {
		return err
	}
	plist, err := agentPlist()
	if err != nil {
		return err
	}
	log := filepath.Join(home, "haas.log")
	text := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>local</string><string>run</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, serviceLabel, xmlText(program), xmlText(log), xmlText(log))
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plist, []byte(text), 0o644); err != nil {
		return err
	}
	exec.Command("launchctl", "bootout", domain(), plist).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain(), plist).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, out)
	}
	return nil
}

func stopService() error {
	plist, err := agentPlist()
	if err != nil {
		return err
	}
	if out, err := exec.Command("launchctl", "bootout", domain(), plist).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootout: %w: %s", err, out)
	}
	return nil
}

func uninstallService() error {
	plist, err := agentPlist()
	if err != nil {
		return err
	}
	exec.Command("launchctl", "bootout", domain(), plist).Run()
	if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// xmlText is text as it may stand in a property list.
func xmlText(text string) string {
	var escaped bytes.Buffer
	xml.EscapeText(&escaped, []byte(text))
	return escaped.String()
}
