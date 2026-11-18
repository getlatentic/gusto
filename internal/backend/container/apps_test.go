//go:build !windows

package container

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// appsOn is the container apps of an organization whose runtime is a script that writes down what it is asked and, for logs,
// says what the app wrote. Sockets are made in a short folder: a unix socket's path may not be long.
func appsOn(t *testing.T, homes string) (runner *sharedApps, folders, calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "runtime")
	body := "#!/bin/sh\necho \"$@\" >> " + calls + "\n[ \"$1\" = logs ] && echo 'Error: Cannot find module server.js'\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sockets, err := os.MkdirTemp("/tmp", "gusto-apps-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockets) })
	folders = filepath.Join(dir, "agents")
	b := New(Config{Org: "acme", Homes: homes, Containers: sandbox.Containers{Runtime: script, Image: "gusto:dev", CPUs: "1", Memory: "1g"},
		AgentFolder: func(instance string) string { return filepath.Join(folders, instance, "workspace") }})
	return &sharedApps{b: b, folder: b.cfg.AgentFolder, sockets: sockets}, folders, calls
}

func TestAnAppIsNotStartedForAnAgentWithNoFolder(t *testing.T) {
	runner, _, calls := appsOn(t, "")
	_, err := runner.Start(context.Background(), apps.App{ID: "0a1b2c3d4e", Instance: "notes", Command: "node server.js", Port: 3000})
	if err == nil || !strings.Contains(err.Error(), "no folder yet") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(calls); err == nil {
		t.Error("a container was asked for")
	}
}

// An app that never answers is failed with what it wrote, and its container and socket's folder are removed.
func TestAnAppThatNeverAnswersIsRemovedAndSaysWhatItWrote(t *testing.T) {
	homes := filepath.Join(t.TempDir(), "computers")
	runner, folders, calls := appsOn(t, homes)
	app := apps.App{ID: "0a1b2c3d4e", Instance: "notes", Owner: "ada@acme.com", Command: "node server.js", Port: 3000}
	if err := os.MkdirAll(runner.folder(app.Instance), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, err := runner.Start(ctx, app)
	if err == nil || !strings.Contains(err.Error(), "did not answer in time") || !strings.Contains(err.Error(), "Cannot find module server.js") {
		t.Fatalf("got %v", err)
	}
	said, _ := os.ReadFile(calls)
	person := computer.ID("acme", "", "ada@acme.com")
	for _, want := range []string{
		"run -d --name gusto-app-acme-0a1b2c3d4e --network=none",
		filepath.Join(folders, "notes", "workspace") + ":/job/workspace",
		filepath.Join(homes, person, "home") + ":" + sandbox.Home,
		"logs --tail 40 gusto-app-acme-0a1b2c3d4e",
	} {
		if !strings.Contains(string(said), want) {
			t.Errorf("want %q in:\n%s", want, said)
		}
	}
	if lines := strings.Split(strings.TrimSpace(string(said)), "\n"); lines[len(lines)-1] != "rm -f gusto-app-acme-0a1b2c3d4e" {
		t.Errorf("the container is removed last: %q", lines)
	}
	if _, err := os.Stat(runner.socketDir(app)); !os.IsNotExist(err) {
		t.Errorf("the socket's folder is left: %v", err)
	}
}

func TestALiveAppAnswersThroughItsSocketByTheHostItListensFor(t *testing.T) {
	runner, _, _ := appsOn(t, "")
	app := apps.App{ID: "0a1b2c3d4e", Host: "localhost:3000", Port: 3000}
	if err := os.MkdirAll(runner.socketDir(app), 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(runner.socketDir(app), "app.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	asked := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked <- r.Host
		w.WriteHeader(http.StatusNotFound)
	})}
	go server.Serve(listener)
	defer server.Close()

	app.Target = apps.SocketTarget(socket)
	if err := runner.Answers(context.Background(), app); err != nil {
		t.Fatalf("an app that answers, even with a 404: %v", err)
	}
	if host := <-asked; host != "localhost:3000" {
		t.Errorf("asked for %q", host)
	}
	if err := runner.Answers(context.Background(), apps.App{Target: "10.0.0.1:80"}); err == nil {
		t.Error("an app with no socket answered")
	}
}

func TestAServerGivenNoAgentsFoldersRunsNoAppsInContainers(t *testing.T) {
	if New(Config{Org: "acme"}).Apps() != nil {
		t.Error("apps were offered with no folder to start them in")
	}
	if New(Config{Org: "acme", AgentFolder: func(string) string { return "/agents" }}).Apps() != nil {
		t.Error("apps were offered with no folder for their sockets")
	}
}

// An app whose socket would be too long a path is not started, and the person is told which setting makes it shorter.
func TestAnAppWhoseSocketPathIsTooLongIsNotStarted(t *testing.T) {
	runner, _, calls := appsOn(t, "")
	runner.sockets = "/" + strings.Repeat("s", socketPathMost)
	app := apps.App{ID: "0a1b2c3d4e", Instance: "notes", Command: "node server.js", Port: 3000}
	if err := os.MkdirAll(runner.folder(app.Instance), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Start(context.Background(), app); err == nil || !strings.Contains(err.Error(), "GUSTO_APP_SOCKETS") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(calls); err == nil {
		t.Error("a container was asked for")
	}
}
