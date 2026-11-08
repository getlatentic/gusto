package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/settings"
)

func TestTheLocalSetupIsMadeOnceAndOnlyItsPersonCanReadIt(t *testing.T) {
	home := filepath.Join(t.TempDir(), "gusto")
	first, err := ensureLocal(home)
	if err != nil || first.Port == 0 || len(first.Secret) != 64 {
		t.Fatalf("%+v, %v", first, err)
	}
	again, err := ensureLocal(home)
	if err != nil || !reflect.DeepEqual(again, first) {
		t.Errorf("the setup changed when it was read again: %+v, %v", again, err)
	}
	for path, want := range map[string]os.FileMode{home: 0o700, filepath.Join(home, "local.json"): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s is %v, want %v (%v)", path, info.Mode().Perm(), want, err)
		}
	}
	os.WriteFile(filepath.Join(home, "local.json"), []byte("{not json"), 0o600)
	if _, err := ensureLocal(home); err == nil {
		t.Error("a damaged setup was used")
	}
}

func TestAServerForOnePersonListensOnTheLoopbackOnly(t *testing.T) {
	for address, want := range map[string]bool{"127.0.0.1:7140": true, "[::1]:7140": true, "0.0.0.0:7140": false, "192.168.1.4:7140": false, ":7140": false, "localhost:7140": false} {
		if got := onLoopback(address); got != want {
			t.Errorf("%s: %v, want %v", address, got, want)
		}
	}
	t.Setenv("GUSTO_LOCAL", "1")
	t.Setenv("GUSTO_PASSWORD", "x")
	t.Setenv("GUSTO_ADDR", "0.0.0.0:7140")
	if code := serve(nil); code != 2 {
		t.Errorf("a server for one person on every address started: %d", code)
	}
}

// A browser that keeps running is left to run, and one that cannot open the link says so, so the link is printed instead.
func TestOpeningALinkDoesNotWaitForTheBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the opener is rundll32")
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	t.Cleanup(func() { browseWait = 3 * time.Second })
	for script, fails := range map[string]bool{"sleep 30": false, "exit 3": true} {
		// A browser that keeps running is waited for a moment, and one that fails is answered as soon as it does, so the wait is long
		// where a failure is expected, and short where it is not.
		browseWait = 5 * time.Second
		if !fails {
			browseWait = 200 * time.Millisecond
		}
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, opener), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		began := time.Now()
		err := browse("http://127.0.0.1:1/local/enter")
		if took := time.Since(began); took > 5*time.Second {
			t.Errorf("%s: opening took %v", script, took)
		}
		if (err != nil) != fails {
			t.Errorf("%s: %v", script, err)
		}
	}
}

// A setup made before gusto had a proxy of its own is given a port for one, kept, and the rest is as it was.
func TestASetupFromBeforeTheProxyIsGivenAPortForIt(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "local.json"), []byte(`{"port": 7140, "secret": "s"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	setup, err := ensureLocal(home)
	if err != nil || setup.Port != 7140 || setup.Secret != "s" || setup.ProxyPort == 0 || setup.ProxyPort == setup.Port {
		t.Fatalf("%+v %v", setup, err)
	}
	if again, _ := ensureLocal(home); again.ProxyPort != setup.ProxyPort {
		t.Errorf("the proxy's port was not kept: %d then %d", setup.ProxyPort, again.ProxyPort)
	}
}

// A person installs an engine into a folder their PATH did not name when gusto was installed, and runs gusto open in that
// terminal: the server looks there from then on.
func TestTheServerLooksWhereThePersonLastRanGustoFrom(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", "/usr/bin")
	if err := (localSetup{Port: 1, Secret: "s", Path: "/opt/engines/bin:/usr/bin"}).save("."); err != nil {
		t.Fatal(err)
	}
	adoptRecordedPath()
	if got := os.Getenv("PATH"); got != "/opt/engines/bin:/usr/bin" {
		t.Errorf("the server looks on %s", got)
	}
}

// An app started from the Dock has a bare PATH, so gusto asks the person's login shell for theirs.
func TestThePersonsOwnShellIsAskedForItsPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows gives a program the person's PATH")
	}
	bin := t.TempDir()
	shell := filepath.Join(bin, "fake-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\necho a greeting the shell prints\nPATH=/opt/the/persons/bin:/usr/bin\neval \"$2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	if got := loginShellPath(); got != "/opt/the/persons/bin:/usr/bin" {
		t.Errorf("the shell's PATH was read as %q", got)
	}
}

// A computer's own choice of wall is the GUSTO_SANDBOX the rest of gusto reads, so it runs tasks behind it, and a person's own setting
// is left alone.
func TestTheWallChosenIsTheSandboxTasksRunBehind(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	// A computer with no docker and no VM has its own sandbox where it has a process sandbox that holds a task to a folder and a
	// port, and nothing otherwise.
	expected := "here"
	if isolation.ProcessWall(isolation.This(isolation.Settings{})).Ready {
		expected = "process"
	}
	nothingSet := func(string) string { return "" }
	if decided, err := chooseWall(nothingSet); err != nil || decided["GUSTO_SANDBOX"] != expected || wall == nil {
		t.Errorf("a computer with no docker and no VM chose %v, want %q: %v", decided, expected, err)
	}
	theirs := func(name string) string { return map[string]string{"GUSTO_SANDBOX": "container"}[name] }
	if decided, err := chooseWall(theirs); err != nil || len(decided) != 0 || !wall.Forced {
		t.Errorf("a person's own setting was second-guessed: %v, %v", decided, err)
	}
}

// The sandbox a person installed is what gusto uses, under what they set themselves, and the sandbox chosen is read over both.
func TestAnInstalledSandboxIsUsedUnderThePersonsOwnSettings(t *testing.T) {
	t.Parallel()
	installed := map[string]string{"GUSTO_OPENSHELL": "/h/openshell/bin/openshell", "GUSTO_OPENSHELL_IMAGE": "/h/openshell/rootfs.tar"}
	theirs := map[string]string{"GUSTO_OPENSHELL_IMAGE": "/theirs/rootfs.tar"}
	lookup := settings.Over(map[string]string{"GUSTO_SANDBOX": "openshell"}, settings.Under(installed, func(name string) string { return theirs[name] }))
	cfg, _, err := loadServe(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.boxes.openShell.cfg; got.Binary != installed["GUSTO_OPENSHELL"] || got.Image != theirs["GUSTO_OPENSHELL_IMAGE"] || cfg.boxes.sandbox != "openshell" {
		t.Errorf("the sandbox is %+v, kind %s", got, cfg.boxes.sandbox)
	}
}

// The server of one person on their own computer is set by gusto: on the loopback, signed in by its own secret, and with the
// person as the admin of their own registry.
func TestTheServerOnAPersonsComputerIsSetByGusto(t *testing.T) {
	t.Parallel()
	setup := localSetup{Secret: "local-secret", Port: 8041, ProxyPort: 8042, FoundKeys: []string{"DEEPSEEK_API_KEY", "OPENAI_API_KEY"}}
	theirs := map[string]string{"GUSTO_ADDR": "0.0.0.0:80", "GUSTO_ORG": "acme"}
	cfg, _, err := loadServe(settings.Over(ownServer("/home/ada/gusto", setup), func(name string) string { return theirs[name] }))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.core.addr != "127.0.0.1:8041" || cfg.core.password != "local-secret" || !cfg.core.local || cfg.core.org != "acme" {
		t.Errorf("core is %+v", cfg.core)
	}
	if !slices.Equal(cfg.access.foundKeys, setup.FoundKeys) {
		t.Errorf("found keys are %v", cfg.access.foundKeys)
	}
}

// Every setting gusto decides for a person's own server is one serve reads, so none is decided and then never read.
func TestEverySettingGustoDecidesLocallyIsOneServeReads(t *testing.T) {
	t.Parallel()
	declared := map[string]bool{}
	for _, each := range cmdline.ParamsOf(new(serveSettings).declare) {
		declared[each.Name] = true
	}
	for name := range ownServer("/home/ada/gusto", localSetup{}) {
		if !declared[name] {
			t.Errorf("%s is decided for the local server, and serve does not read it", name)
		}
	}
}
