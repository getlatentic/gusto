package container

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/sandbox"
)

// A home an app runs on is not reset, and one whose app has stopped is; a home a person left waits for their app to stop.
func TestAHomeAnAppRunsOnIsNotRemovedUnderIt(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, home := theHome(t, b, root)
	running := true
	b.homes.appsOn = func(asked string) (bool, error) {
		if asked != id {
			t.Errorf("asked about the apps on %q, not %q", asked, id)
		}
		return running, nil
	}

	if err := b.ResetHome(testPerson, ""); !errors.Is(err, ErrHomeInApp) || !exists(home) {
		t.Fatalf("resetting a home an app runs on answered %v, and the home is there: %v", err, exists(home))
	}
	os.WriteFile(b.homes.markFile(id), nil, 0o600)
	b.homes.removeMarked()
	if !exists(home) || !exists(b.homes.markFile(id)) {
		t.Fatal("the home of a person who left was removed under their app, or its mark was cleared")
	}

	running = false
	b.homes.removeMarked()
	if exists(home) || exists(b.homes.markFile(id)) {
		t.Error("the home of a person who left stayed after their app stopped")
	}
}

// A home is held from when an app's container is asked for until it runs, when the runtime can say so; a home being removed is
// not opened for an app.
func TestAHomeIsHeldWhileAnAppStartsOnIt(t *testing.T) {
	b, root := homeBackend(t, 1<<20, fixed(100))
	id, _ := theHome(t, b, root)
	b.homes.appsOn = func(string) (bool, error) { return false, nil }

	dir, release, err := b.homes.openForApp(id)
	if err != nil || dir != b.homes.dir(id) {
		t.Fatalf("the home was not opened for an app: %q %v", dir, err)
	}
	if err := b.ResetHome(testPerson, ""); !errors.Is(err, ErrHomeInApp) {
		t.Errorf("resetting a home an app is starting on answered %v", err)
	}
	release()
	if err := b.ResetHome(testPerson, ""); err != nil {
		t.Fatalf("a home whose app started and stopped could not be reset: %v", err)
	}

	b.homes.removing[id] = true
	if _, _, err := b.homes.openForApp(id); err == nil {
		t.Error("a home being removed was opened for an app")
	}
}

// The backend asks the runtime for the running containers of apps with the person's home, in this organization.
func TestTheBackendAsksTheRuntimeWhetherAnAppRunsOnAHome(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "runtime")
	body := "#!/bin/sh\necho \"$@\" >> " + calls + "\n[ \"$1\" = ps ] && echo 4f2a9c1b\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	b := New(Config{Org: testOrg, Homes: filepath.Join(dir, "computers"), Containers: sandbox.Containers{Runtime: script},
		AgentFolder: func(string) string { return dir }})

	running, err := b.appRunsOn("0a1b2c3d")
	said, _ := os.ReadFile(calls)
	want := "ps -q --filter label=" + sandbox.LabelPrefix + "app --filter label=" + sandbox.LabelPrefix + "person=0a1b2c3d --filter label=" +
		sandbox.LabelPrefix + "org=" + testOrg
	if err != nil || !running || strings.TrimSpace(string(said)) != want {
		t.Fatalf("running %v, %v, asked:\n%s", running, err, said)
	}
	if New(Config{Org: testOrg, Homes: dir}).homes.appsOn != nil {
		t.Error("a server that runs no apps asks the runtime about them")
	}
}
