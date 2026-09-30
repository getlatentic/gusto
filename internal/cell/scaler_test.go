package cell

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeHosts struct {
	hosts      []Instance
	launched   int
	terminated []string
	failLaunch error
	next       string
}

func (f *fakeHosts) Launch() (string, error) {
	if f.failLaunch != nil {
		return "", f.failLaunch
	}
	f.launched++
	f.hosts = append(f.hosts, Instance{ID: f.next, Launched: clockNow})
	return f.next, nil
}

func (f *fakeHosts) Terminate(id string) error {
	f.terminated = append(f.terminated, id)
	for i, host := range f.hosts {
		if host.ID == id {
			f.hosts = append(f.hosts[:i], f.hosts[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeHosts) Running() ([]Instance, error) { return append([]Instance(nil), f.hosts...), nil }

var clockNow = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

type rig struct {
	t       *testing.T
	scaler  *Scaler
	hosts   *fakeHosts
	waiting int
	now     time.Time
}

func scaling(t *testing.T) *rig {
	t.Helper()
	r := &rig{t: t, hosts: &fakeHosts{next: "i-0aaa1111"}, now: clockNow}
	pool := &Pool{Dir: filepath.Join(t.TempDir(), "cells"), Secret: secret, MaxAge: time.Hour, MinFreeMiB: 0}
	r.scaler = &Scaler{Pool: pool, Launcher: r.hosts, Waiting: func() int { return r.waiting }, Max: 2,
		Idle: 10 * time.Minute, Boot: 10 * time.Minute, Backoff: time.Minute}
	r.scaler.now = func() time.Time { clockNow = r.now; return r.now }
	return r
}

// cell registers a host as a cell, with a server on it or not.
func (r *rig) cell(id string, withServer bool) *agentHost {
	r.t.Helper()
	name := "cell-" + strings.TrimPrefix(id, "i-")
	h := newAgent(r.t, name, 9000)
	if withServer {
		if err := h.servers.Start(fakeOrg(r.t, "acme-com", freePort(r.t))); err != nil {
			r.t.Fatal(err)
		}
	}
	if err := Register(r.scaler.Pool.Dir, h.registration(name, r.now)); err != nil {
		r.t.Fatal(err)
	}
	return h
}

func (r *rig) tick(after time.Duration) { r.now = r.now.Add(after); r.scaler.Tick() }

func TestNothingIsStartedWhileNothingWaits(t *testing.T) {
	r := scaling(t)
	r.tick(0)
	r.tick(time.Hour)
	if r.hosts.launched != 0 {
		t.Errorf("%d hosts were started for nobody", r.hosts.launched)
	}
}

func TestOneHostIsStartedForWhoeverWaitsAndNotAnotherWhileItIsOnItsWay(t *testing.T) {
	r := scaling(t)
	r.waiting = 3
	r.tick(0)
	r.tick(30 * time.Second)
	r.tick(2 * time.Minute)
	if r.hosts.launched != 1 {
		t.Errorf("one host for however many wait, and no more while it starts: %d", r.hosts.launched)
	}
	// It is a cell now, and they still wait, since it has not room: another may start, up to the most there may be.
	r.cell("i-0aaa1111", true)
	r.hosts.next = "i-0bbb2222"
	r.tick(time.Minute)
	if r.hosts.launched != 2 {
		t.Errorf("a cell that cannot take them all is joined by another: %d", r.hosts.launched)
	}
	r.cell("i-0bbb2222", true)
	r.hosts.next = "i-0ccc3333"
	r.tick(time.Minute)
	if r.hosts.launched != 2 {
		t.Errorf("there is a most: %d", r.hosts.launched)
	}
}

func TestAHostThatCouldNotBeStartedIsNotTriedAgainAtOnce(t *testing.T) {
	r := scaling(t)
	r.waiting = 1
	r.hosts.failLaunch = errors.New("no capacity")
	r.tick(0)
	r.hosts.failLaunch = nil
	r.tick(30 * time.Second)
	if r.hosts.launched != 0 {
		t.Error("tried again inside the backoff")
	}
	r.tick(time.Minute)
	if r.hosts.launched != 1 {
		t.Errorf("not tried again after it: %d", r.hosts.launched)
	}
}

func TestAHostThatNeverBecomesACellIsStoppedAfterItsTime(t *testing.T) {
	r := scaling(t)
	r.waiting = 1
	r.tick(0)
	r.tick(9 * time.Minute)
	if len(r.hosts.terminated) != 0 {
		t.Errorf("a host that is still starting was stopped: %v", r.hosts.terminated)
	}
	r.tick(2 * time.Minute)
	if len(r.hosts.terminated) != 1 || r.hosts.terminated[0] != "i-0aaa1111" {
		t.Errorf("%v", r.hosts.terminated)
	}
}

func TestACellWithNothingOnItIsStoppedAfterItHasBeenIdleAndNotBefore(t *testing.T) {
	r := scaling(t)
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now}}
	busy := r.cell("i-0aaa1111", false)
	r.tick(0)
	r.tick(9 * time.Minute)
	if len(r.hosts.terminated) != 0 {
		t.Fatalf("stopped inside the idle time: %v", r.hosts.terminated)
	}
	if err := busy.servers.Start(fakeOrg(t, "acme-com", freePort(t))); err != nil {
		t.Fatal(err)
	}
	r.tick(2 * time.Minute)
	r.tick(9 * time.Minute)
	if len(r.hosts.terminated) != 0 {
		t.Errorf("a cell with an organisation on it was stopped, or its idle time was not reset: %v", r.hosts.terminated)
	}
	for _, slug := range busy.servers.Running() {
		busy.servers.Stop(slug, time.Second)
	}
	r.tick(time.Minute)
	r.tick(11 * time.Minute)
	if len(r.hosts.terminated) != 1 || r.hosts.terminated[0] != "i-0aaa1111" {
		t.Errorf("an idle cell is stopped: %v", r.hosts.terminated)
	}
}

func TestAnIdleCellIsKeptWhileAnOrganisationWaitsForRoom(t *testing.T) {
	r := scaling(t)
	r.hosts.hosts = []Instance{{ID: "i-0aaa1111", Launched: r.now}}
	r.cell("i-0aaa1111", false)
	r.waiting = 1
	r.hosts.next = "i-0bbb2222"
	r.tick(0)
	r.tick(time.Hour)
	if slices.Contains(r.hosts.terminated, "i-0aaa1111") {
		t.Errorf("a cell was stopped while somebody waited: %v", r.hosts.terminated)
	}
}

func TestACellThatIsNotOneOfTheScalersIsNeverStopped(t *testing.T) {
	r := scaling(t)
	r.cell("i-0ddd4444", false)
	r.tick(0)
	r.tick(time.Hour)
	if len(r.hosts.terminated) != 0 {
		t.Errorf("a cell somebody else started was stopped: %v", r.hosts.terminated)
	}
}

func TestHostsAreStartedFromATemplateWithTheScriptFilledInAndNothingElseIsAccepted(t *testing.T) {
	script := filepath.Join(t.TempDir(), "user-data.sh")
	os.WriteFile(script, []byte("ROLE=@ROLE@ RUN=@RUN@ RELEASE=@RELEASE@\n"), 0o644)
	var ran [][]string
	var seen string
	e := &EC2{Template: "haas-poc-demo-host-cell", UserData: script, Settings: map[string]string{"ROLE": "cell", "RUN": "live", "RELEASE": "abc123"},
		Run: func(args ...string) ([]byte, error) {
			ran = append(ran, args)
			for i, arg := range args {
				if arg == "--user-data" {
					text, _ := os.ReadFile(strings.TrimPrefix(args[i+1], "file://"))
					seen = string(text)
				}
			}
			if args[1] == "describe-instances" {
				return []byte("i-0aaa1111\t2026-10-01T09:00:00+00:00\ni-0bbb2222\t2026-10-01T09:05:00.000Z\nnot a line\n"), nil
			}
			return []byte("i-0aaa1111\n"), nil
		}}
	id, err := e.Launch()
	if err != nil || id != "i-0aaa1111" || seen != "ROLE=cell RUN=live RELEASE=abc123\n" {
		t.Fatalf("%q %v %q", id, err, seen)
	}
	args := strings.Join(ran[0], " ")
	for _, want := range []string{"LaunchTemplateName=haas-poc-demo-host-cell,Version=$Latest", "Key=project,Value=haas-poc", "Key=scaler,Value=on"} {
		if !strings.Contains(args, want) {
			t.Errorf("%q is not in %s", want, args)
		}
	}
	hosts, err := e.Running()
	if err != nil || len(hosts) != 2 || hosts[0].ID != "i-0aaa1111" || hosts[1].Launched.Minute() != 5 {
		t.Errorf("%+v %v", hosts, err)
	}
	if !strings.Contains(strings.Join(ran[1], " "), "Name=tag:scaler,Values=on") {
		t.Errorf("only the scaler's own hosts are listed: %v", ran[1])
	}
	for _, bad := range []string{"", "i-", "--help", "i-0aaa1111 --dry-run", "x-0aaa1111", "i-ZZZZZZZZ"} {
		if err := e.Terminate(bad); err == nil {
			t.Errorf("%q was taken for a host", bad)
		}
	}
	if err := e.Terminate("i-0aaa1111"); err != nil {
		t.Error(err)
	}
	os.WriteFile(script, []byte("ROLE=@ROLE@ MORE=@NOBODY_SAID@\n"), 0o644)
	if _, err := e.Launch(); err == nil || !strings.Contains(err.Error(), "@NOBODY_SAID@") {
		t.Errorf("a script with a blank in it was launched: %v", err)
	}
	e.Run = func(...string) ([]byte, error) { return []byte("garbage\n"), nil }
	os.WriteFile(script, []byte("x\n"), 0o644)
	if _, err := e.Launch(); err == nil {
		t.Error("an answer that is not a host was taken for one")
	}
}

// freePort is a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
