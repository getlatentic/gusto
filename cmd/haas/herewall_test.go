package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/seatbelt"
)

// An agent that tries to read the person's secrets, write outside its folder and reach the network is held, and one that works in its
// own folder is not. The folders are in the real home, since a wall around a temporary folder is a different thing.
func TestAnAgentBehindTheSandboxIsHeldToItsOwnFolder(t *testing.T) {
	if runtime.GOOS != "darwin" || seatbelt.Available() != nil {
		t.Skip("needs macOS's sandbox-exec")
	}
	person, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	base, err := os.MkdirTemp(person, ".haas-herewall-")
	if err != nil {
		t.Skip("the home cannot be written")
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	base, _ = filepath.EvalSymlinks(base)

	dir := job.Dir(filepath.Join(base, "jobs", "one"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	home, data := filepath.Join(dir.Path(), "home"), filepath.Join(base, "sessions", "one", "state")
	for _, folder := range []string{home, data} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// What the person has that the agent must not reach: haas's own secret, beside the jobs, and a key in their home.
	secret := filepath.Join(base, "local.json")
	key := filepath.Join(base, "id_ed25519")
	os.WriteFile(secret, []byte(`{"secret":"s"}`), 0o600)
	os.WriteFile(key, []byte("a private key"), 0o600)
	intruder := filepath.Join(base, "written-by-the-agent")

	proxy, _ := net.Listen("tcp", "127.0.0.1:0")
	defer proxy.Close()
	other, _ := net.Listen("tcp", "127.0.0.1:0")
	defer other.Close()
	port := func(l net.Listener) string { return strconv.Itoa(l.Addr().(*net.TCPAddr).Port) }

	wrap, err := behindSeatbelt(dir, job.Spec{Harness: "opencode"}, home, data, port(proxy))
	if err != nil {
		t.Fatal(err)
	}
	script := `echo mine > made.txt && echo "workspace: written"
echo state > ` + data + `/state && echo "state: written"
cat ` + secret + ` >/dev/null 2>&1 && echo "haas secret: READ" || echo "haas secret: refused"
cat ` + key + ` >/dev/null 2>&1 && echo "key: READ" || echo "key: refused"
echo x > ` + intruder + ` 2>/dev/null; [ -e ` + intruder + ` ] && echo "home: WRITTEN" || echo "home: refused"
echo x > ` + secret + ` 2>/dev/null
nc -z -w 2 127.0.0.1 ` + port(proxy) + ` && echo "proxy: reached" || echo "proxy: REFUSED"
nc -z -w 2 127.0.0.1 ` + port(other) + ` && echo "another local service: REACHED" || echo "another local service: refused"
nc -z -w 2 1.1.1.1 443 && echo "internet: REACHED" || echo "internet: refused"`
	command := exec.Command("/bin/sh", "-c", script)
	command.Dir = dir.Workspace()
	wrap(command)
	out, _ := command.CombinedOutput()
	for _, want := range []string{"workspace: written", "state: written", "haas secret: refused", "key: refused", "home: refused",
		"proxy: reached", "another local service: refused", "internet: refused"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%q is not in:\n%s", want, out)
		}
	}
	if got := string(mustRead(t, secret)); got != `{"secret":"s"}` {
		t.Errorf("the agent overwrote haas's secret with %q", got)
	}
	if _, err := os.Stat(intruder); err == nil {
		t.Error("the agent wrote a file in the person's home")
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(dir.Workspace(), "made.txt"))), "mine") {
		t.Error("the agent's work in its own folder is not there")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
