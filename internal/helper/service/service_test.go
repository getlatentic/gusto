package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type calls struct {
	made  []string
	fails map[string]bool
}

func (c *calls) run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	c.made = append(c.made, line)
	for prefix := range c.fails {
		if strings.HasPrefix(line, prefix) {
			return "it said no", errors.New("exit 1")
		}
	}
	return "", nil
}

func spec(t *testing.T) Spec {
	t.Helper()
	return Spec{Program: "/usr/local/bin/gusto-helper", Server: "https://gusto.example.com/?a=1&b=2", Home: t.TempDir()}
}

func TestOnMacOSTheHelperIsALaunchAgentStartedNowAndAtLogin(t *testing.T) {
	c, s := &calls{}, spec(t)
	said, err := Manager{GOOS: "darwin", Run: c.run, Uid: 501}.Install(s)
	if err != nil || !strings.Contains(said, "every login") {
		t.Fatalf("%q %v", said, err)
	}
	plist, err := os.ReadFile(filepath.Join(s.Home, "Library", "LaunchAgents", "com.latentic.gusto-helper.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>" + s.Program + "</string>", "<string>run</string>", "https://gusto.example.com/?a=1&amp;b=2", "<key>RunAtLoad</key><true/>", "<key>KeepAlive</key><true/>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("the plist lacks %s:\n%s", want, plist)
		}
	}
	if len(c.made) != 2 || !strings.HasPrefix(c.made[0], "launchctl bootout gui/501/com.latentic.gusto-helper") || !strings.HasPrefix(c.made[1], "launchctl bootstrap gui/501 ") {
		t.Errorf("launchctl was asked %q: the old one has to come out before the new one goes in", c.made)
	}
	if err := (Manager{GOOS: "darwin", Run: c.run, Uid: 501}).Uninstall(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Home, "Library", "LaunchAgents", "com.latentic.gusto-helper.plist")); err == nil {
		t.Error("uninstalling left the launch agent")
	}
}

func TestOnLinuxTheHelperIsAUserUnitAndTheCommandLineIsQuoted(t *testing.T) {
	c := &calls{}
	s := Spec{Program: "/home/ada/my tools/gusto-helper", Server: "https://gusto.example.com/100%", Home: t.TempDir()}
	if _, err := (Manager{GOOS: "linux", Run: c.run}).Install(s); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(s.Home, ".config", "systemd", "user", "gusto-helper.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), `ExecStart="/home/ada/my tools/gusto-helper" run -server "https://gusto.example.com/100%%"`) || !strings.Contains(string(unit), "Restart=always") {
		t.Errorf("the unit is:\n%s", unit)
	}
	if strings.Join(c.made, "|") != "systemctl --user daemon-reload|systemctl --user enable --now gusto-helper.service" {
		t.Errorf("systemctl was asked %q", c.made)
	}
}

func TestOnWindowsTheHelperIsATaskAtLogonThatRunsAsThePersonAndAnInstallReplacesTheLast(t *testing.T) {
	c := &calls{}
	s := Spec{Program: `C:\Users\Ada\My Tools\gusto-helper.exe`, Server: "https://gusto.example.com"}
	if _, err := (Manager{GOOS: "windows", Run: c.run}).Install(s); err != nil {
		t.Fatal(err)
	}
	create := c.made[0]
	for _, want := range []string{"/Create", "/F", "ONLOGON", "LIMITED", "/TN gusto-helper", `"C:\Users\Ada\My Tools\gusto-helper.exe" run -server "https://gusto.example.com"`} {
		if !strings.Contains(create, want) {
			t.Errorf("the task lacks %s: %s", want, create)
		}
	}
	if !strings.HasPrefix(c.made[1], "schtasks /Run /TN gusto-helper") {
		t.Errorf("the task was not started: %q", c.made)
	}
}

func TestRemovingAServiceThatWasNeverInstalledIsNotAnError(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		c := &calls{fails: map[string]bool{"schtasks /Query": true, "launchctl": true, "systemctl": true}}
		if err := (Manager{GOOS: goos, Run: c.run}).Uninstall(spec(t)); err != nil {
			t.Errorf("%s: %v", goos, err)
		}
	}
}

func TestAnInstallThatTheSystemRefusesSaysWhatItSaid(t *testing.T) {
	c := &calls{fails: map[string]bool{"launchctl bootstrap": true}}
	if _, err := (Manager{GOOS: "darwin", Run: c.run, Uid: 501}).Install(spec(t)); err == nil || !strings.Contains(err.Error(), "it said no") {
		t.Errorf("%v", err)
	}
}

func TestTheHelperHasToBeNamedByItsWholePathAndAServer(t *testing.T) {
	m := Manager{GOOS: "linux", Run: (&calls{}).run}
	if _, err := m.Install(Spec{Program: "gusto-helper", Server: "https://x"}); err == nil {
		t.Error("a relative program was installed")
	}
	if _, err := m.Install(Spec{Program: "/usr/bin/gusto-helper"}); err == nil {
		t.Error("a service with no server was installed")
	}
}
