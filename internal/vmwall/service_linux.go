package vmwall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func userUnit(l Layout) string {
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(l.User, ".config")
	}
	return filepath.Join(config, "systemd", "user", "haas-openshell.service")
}

// startGateway makes the gateway a systemd unit of this person's: started at login, started again if it fails, and started now.
func startGateway(l Layout) error {
	var env strings.Builder
	pairs := l.GatewayEnv()
	sort.Strings(pairs)
	for _, pair := range pairs {
		fmt.Fprintf(&env, "Environment=%q\n", pair)
	}
	text := fmt.Sprintf(`[Unit]
Description=OpenShell gateway for haas, the virtual machines agents run in

[Service]
ExecStart=%q
%sRestart=on-failure
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`, l.Program(Gateway), env.String(), l.Log(), l.Log())
	unit := userUnit(l)
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unit, []byte(text), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"--user", "daemon-reload"}, {"--user", "enable", "haas-openshell.service"}, {"--user", "restart", "haas-openshell.service"}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %v: %w: %s", args[1:], err, out)
		}
	}
	return nil
}

// stopGateway ends the gateway and takes the unit away.
func stopGateway(l Layout) error {
	exec.Command("systemctl", "--user", "disable", "--now", "haas-openshell.service").Run()
	if err := os.Remove(userUnit(l)); err != nil && !os.IsNotExist(err) {
		return err
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}
