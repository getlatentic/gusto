package vmwall

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const serviceLabel = "com.latentic.gusto.openshell"

func agentPlist(l Layout) string {
	return filepath.Join(l.User, "Library", "LaunchAgents", serviceLabel+".plist")
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

func xmlText(text string) string {
	var escaped bytes.Buffer
	xml.EscapeText(&escaped, []byte(text))
	return escaped.String()
}

// startGateway makes the gateway a launch agent of this person's: started at login, started again if it stops, and started now.
func startGateway(l Layout) error {
	var env strings.Builder
	pairs := l.GatewayEnv()
	sort.Strings(pairs)
	for _, pair := range pairs {
		name, value, _ := strings.Cut(pair, "=")
		fmt.Fprintf(&env, "    <key>%s</key><string>%s</string>\n", xmlText(name), xmlText(value))
	}
	text := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string></array>
  <key>EnvironmentVariables</key>
  <dict>
%s  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, serviceLabel, xmlText(l.Program(Gateway)), env.String(), xmlText(l.Log()), xmlText(l.Log()))
	plist := agentPlist(l)
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

// stopGateway ends the gateway and takes the launch agent away.
func stopGateway(l Layout) error {
	plist := agentPlist(l)
	exec.Command("launchctl", "bootout", domain(), plist).Run()
	if err := os.Remove(plist); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
