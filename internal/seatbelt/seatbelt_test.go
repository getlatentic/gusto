package seatbelt

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// behind runs a shell command behind a policy, as a job's engine is run, and says what it printed.
func behind(t *testing.T, policy Policy, script string) string {
	t.Helper()
	if runtime.GOOS != "darwin" || Available() != nil {
		t.Skip("needs macOS's sandbox-exec")
	}
	profile := filepath.Join(t.TempDir(), "job.sb")
	if err := os.WriteFile(profile, []byte(policy.Profile()), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-c", script)
	Wrap(profile)(command)
	out, _ := command.CombinedOutput()
	return string(out)
}

// A person's home, with a folder the job was given, a secret of theirs, and a tool the engine is made of.
func person(t *testing.T) (home string, policy Policy) {
	t.Helper()
	// A home that is not under a temporary folder, which a job is let write.
	real, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	home, err = os.MkdirTemp(real, ".gusto-seatbelt-test-")
	if err != nil {
		t.Skip("the home cannot be written")
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	if home, err = filepath.EvalSymlinks(home); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{".ssh", "job/workspace", "job/home", "tools/engine/bin"} {
		if err := os.MkdirAll(filepath.Join(home, folder), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("a private key"), 0o600)
	os.WriteFile(filepath.Join(home, "notes.txt"), []byte("my notes"), 0o600)
	os.WriteFile(filepath.Join(home, "tools", "engine", "bin", "engine"), []byte("the engine"), 0o755)
	return home, Policy{Home: home, Writable: []string{filepath.Join(home, "job")}, Readable: []string{filepath.Join(home, "tools", "engine")}}
}

func TestAJobWritesItsOwnFoldersAndNothingElseOfThePersons(t *testing.T) {
	home, policy := person(t)
	said := behind(t, policy, `echo made > `+home+`/job/workspace/made.txt && echo "own folder: written"; echo x > `+home+`/elsewhere.txt 2>/dev/null; [ -e `+home+`/elsewhere.txt ] && echo "home: WRITTEN" || echo "home: refused"; echo x > `+home+`/.ssh/authorized_keys 2>/dev/null; [ -e `+home+`/.ssh/authorized_keys ] && echo "ssh: WRITTEN" || echo "ssh: refused"; echo y > $TMPDIR/scratch && echo "temp: written"`)
	for _, want := range []string{"own folder: written", "home: refused", "ssh: refused", "temp: written"} {
		if !strings.Contains(said, want) {
			t.Errorf("%q is not in:\n%s", want, said)
		}
	}
}

func TestAJobReadsNoneOfThePersonsHomeButItsOwnFoldersAndItsEngine(t *testing.T) {
	home, policy := person(t)
	said := behind(t, policy, `cat `+home+`/.ssh/id_ed25519 >/dev/null 2>&1 && echo "ssh key: READ" || echo "ssh key: refused"; cat `+home+`/notes.txt >/dev/null 2>&1 && echo "notes: READ" || echo "notes: refused"; ls `+home+` >/dev/null 2>&1 && echo "home listing: READ" || echo "home listing: refused"; cat `+home+`/tools/engine/bin/engine >/dev/null 2>&1 && echo "engine: read" || echo "engine: REFUSED"; echo hi > `+home+`/job/workspace/a && cat `+home+`/job/workspace/a >/dev/null && echo "own folder: read"; cat /etc/hosts >/dev/null && echo "system: read"`)
	for _, want := range []string{"ssh key: refused", "notes: refused", "home listing: refused", "engine: read", "own folder: read", "system: read"} {
		if !strings.Contains(said, want) {
			t.Errorf("%q is not in:\n%s", want, said)
		}
	}
}

func TestAJobReachesOnlyGustosProxy(t *testing.T) {
	_, policy := person(t)
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	policy.ProxyPort = proxy.Addr().(*net.TCPAddr).Port
	port := func(l net.Listener) string { return strconv.Itoa(l.Addr().(*net.TCPAddr).Port) }
	said := behind(t, policy, `nc -z -w 2 127.0.0.1 `+port(proxy)+` && echo "proxy: reached" || echo "proxy: REFUSED"; nc -z -w 2 127.0.0.1 `+port(other)+` && echo "another local port: REACHED" || echo "another local port: refused"; nc -z -w 2 1.1.1.1 443 && echo "internet: REACHED" || echo "internet: refused"`)
	for _, want := range []string{"proxy: reached", "another local port: refused", "internet: refused"} {
		if !strings.Contains(said, want) {
			t.Errorf("%q is not in:\n%s", want, said)
		}
	}
}

func TestANetworkLeftOpenIsLeftOpen(t *testing.T) {
	_, policy := person(t)
	policy.ProxyPort = 0
	if strings.Contains(policy.Profile(), "network") {
		t.Errorf("the profile says something of the network:\n%s", policy.Profile())
	}
}

func TestTheEngineOfAnInstalledProgramIsWhatItIsMadeOf(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(home, ".nvm", "lib", "node_modules", "opencode-ai", "bin")
	bin := filepath.Join(home, ".nvm", "bin")
	os.MkdirAll(lib, 0o755)
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(lib, "opencode.js"), []byte("#!/usr/bin/env node"), 0o755)
	os.Symlink(filepath.Join(lib, "opencode.js"), filepath.Join(bin, "opencode"))
	got := Readable(filepath.Join(bin, "opencode"), home)
	want := []string{lib, filepath.Join(home, ".nvm", "lib", "node_modules")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("an engine installed with npm is read from %v, want %v", got, want)
	}
	if outside := Readable("/bin/sh", home); len(outside) != 0 {
		t.Errorf("a system program asks for %v of the person's home", outside)
	}
}
