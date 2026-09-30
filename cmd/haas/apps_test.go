package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/apps"
	"github.com/getlatentic/haas/internal/computer"
	"github.com/getlatentic/haas/internal/openshell"
)

const appsInstance = "20260930-1200-notes"

// anAppsHost is a gateway that answers as the given status, a computer for ada in it with the
// directory her agent works in, and what runs apps there.
func anAppsHost(t *testing.T, status int) (*sharedApps, string, computer.Computer) {
	t.Helper()
	root := withGateway(t)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The gateway answers only by the name it gave the app.
		if !strings.HasPrefix(r.Host, "default--") || !strings.Contains(r.Host, "--a0123456789.openshell.localhost:") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		w.Write([]byte("hello"))
	}))
	t.Cleanup(gateway.Close)
	t.Setenv("OPENSHELL_FAKE_GATEWAY_PORT", strings.TrimPrefix(gateway.URL, "http://127.0.0.1:"))
	registry, err := computer.Open(filepath.Join(t.TempDir(), "computers"), "acme-com")
	if err != nil {
		t.Fatal(err)
	}
	held, _, err := registry.Ensure("ada@acme.com", "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, held.Sandbox)+openshell.InstanceDir(appsInstance), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := apps.Open(filepath.Join(t.TempDir(), "apps"))
	if err != nil {
		t.Fatal(err)
	}
	return newSharedApps(openshell.Config{Binary: "openshell"}, registry, store), root, held
}

func killAppOnCleanup(t *testing.T, root, sandbox, id string) {
	t.Cleanup(func() {
		if text, err := os.ReadFile(filepath.Join(root, sandbox, "sandbox", "apps", id+".pid")); err == nil {
			if pid, _ := strconv.Atoi(strings.TrimSpace(string(text))); pid > 0 {
				syscall.Kill(-pid, syscall.SIGKILL)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
}

func TestAnAppIsStartedExposedAndSaidToAnswerOnlyOnceItDoes(t *testing.T) {
	runner, root, held := anAppsHost(t, 200)
	app := apps.App{ID: "0123456789", Instance: appsInstance, Owner: "ada@acme.com", Command: "exec sleep 60", Port: 8080}
	killAppOnCleanup(t, root, held.Sandbox, app.ID)

	started, err := runner.Start(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	port := os.Getenv("OPENSHELL_FAKE_GATEWAY_PORT")
	if started.Sandbox != held.Sandbox || started.Host != "default--"+held.Sandbox+"--a0123456789.openshell.localhost:"+port || started.Target != "127.0.0.1:"+port {
		t.Errorf("%+v", started)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "exposed.log")); !strings.Contains(string(text), held.Sandbox+" 8080 a0123456789") {
		t.Errorf("the port was exposed under the app's name: %q", text)
	}
	if _, err := os.Stat(filepath.Join(root, held.Sandbox, "sandbox", "apps", app.ID+".pid")); err != nil {
		t.Errorf("the app was not started: %v", err)
	}

	if err := runner.Stop(context.Background(), started); err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "unexposed.log")); !strings.Contains(string(text), held.Sandbox+" a0123456789") {
		t.Errorf("its address was not taken away: %q", text)
	}
	if _, err := os.Stat(filepath.Join(root, held.Sandbox, "sandbox", "apps", app.ID+".pid")); err == nil {
		t.Error("its process was not stopped")
	}
}

func TestAnAppThatNeverAnswersIsStoppedAndTheErrorSaysWhatItWrote(t *testing.T) {
	runner, root, held := anAppsHost(t, 503)
	app := apps.App{ID: "0123456789", Instance: appsInstance, Owner: "ada@acme.com", Command: "echo 'Error: Cannot find module express'; exec sleep 60", Port: 8080}
	killAppOnCleanup(t, root, held.Sandbox, app.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	_, err := runner.Start(ctx, app)
	if err == nil || !strings.Contains(err.Error(), "did not answer") || !strings.Contains(err.Error(), "Cannot find module express") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, held.Sandbox, "sandbox", "apps", app.ID+".pid")); err == nil {
		t.Error("an app that did not answer was left running")
	}
	if text, _ := os.ReadFile(filepath.Join(root, "unexposed.log")); !strings.Contains(string(text), "a0123456789") {
		t.Errorf("an app that did not answer was left exposed: %q", text)
	}
}

func TestAnAppOfAPersonWithNoComputerIsRefused(t *testing.T) {
	runner, _, _ := anAppsHost(t, 200)
	if _, err := runner.Start(context.Background(), apps.App{ID: "0123456789", Instance: appsInstance, Owner: "bob@acme.com", Command: "true", Port: 8080}); err == nil || !strings.Contains(err.Error(), "no computer") {
		t.Errorf("%v", err)
	}
}

func TestAComputerWithAnAppPeopleUseIsNotIdle(t *testing.T) {
	runner, _, _ := anAppsHost(t, 200)
	live, _ := runner.store.New(appsInstance, "Ada@Acme.com", "node a.js", 8080)
	if !runner.serving("ada@acme.com") || runner.serving("bob@acme.com") {
		t.Fatal("an app that is starting is one somebody waits for")
	}
	runner.store.Change(live.ID, func(a *apps.App) { a.State = apps.Live })
	if !runner.serving("ada@acme.com") {
		t.Error("a live app is used")
	}
	runner.store.Change(live.ID, func(a *apps.App) { a.State = apps.Stopped })
	if runner.serving("ada@acme.com") {
		t.Error("a stopped app keeps a computer awake")
	}
}

// A computer nobody used for a while is stopped, and an app is started in it all the same.
func TestAnAppIsStartedInAComputerThatWasStopped(t *testing.T) {
	runner, root, held := anAppsHost(t, 200)
	app := apps.App{ID: "0123456789", Instance: appsInstance, Owner: "ada@acme.com", Command: "exec sleep 60", Port: 8080}
	killAppOnCleanup(t, root, held.Sandbox, app.ID)
	stopped := filepath.Join(root, held.Sandbox, "stopped")
	os.WriteFile(stopped, nil, 0o644)

	if _, err := runner.Start(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stopped); err == nil {
		t.Error("the computer was not started")
	}
}
