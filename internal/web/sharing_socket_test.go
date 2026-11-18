package web

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/apps"
)

// appOnSocket serves an app on a unix socket, as an app's container does, and says the host each request named. The socket's
// path is short because a unix socket's may not be long, and a test's own folder on a Mac is.
func appOnSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "gusto-app-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "app.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("in a container, asked for " + r.Host + " " + r.URL.RequestURI()))
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return socket
}

func TestAnAppInAContainerIsReachedThroughItsSocket(t *testing.T) {
	site, runner, _ := sharingSite(t)
	runner.socket = appOnSocket(t)
	id := sharedApp(t, site, startedInstance(t, site))

	answer := site.ask(t, "GET", "/apps/"+id+"/notes?x=1", nil)
	if answer.Code != http.StatusOK || answer.Body.String() != "in a container, asked for localhost:8080 /notes?x=1" {
		t.Fatalf("the app answered %d: %s", answer.Code, answer.Body)
	}
}

// A live app that does not answer when the server starts, because its container ended with the server before, is started
// again with the same id, so the address people were given still opens it. One that answers is left alone.
func TestARestartStartsALiveAppAgainThatNoLongerAnswers(t *testing.T) {
	site, runner, _ := sharingSite(t)
	app := liveApp(t, site, "node a.js")
	runner.silent = errors.New("connection refused")
	runner.socket = appOnSocket(t)
	site.server.ReconcileApps()
	site.server.Wait()

	again, _ := site.server.apps.Get(app.ID)
	if again.State != apps.Live || again.Target != apps.SocketTarget(runner.socket) || len(runner.started) != 1 {
		t.Fatalf("an app that stopped with the server: %+v, started %d times", again, len(runner.started))
	}
	if answer := site.ask(t, "GET", "/apps/"+app.ID+"/", nil); answer.Code != http.StatusOK {
		t.Errorf("the app started again answered %d", answer.Code)
	}

	runner.silent = nil
	site.server.ReconcileApps()
	site.server.Wait()
	if len(runner.started) != 1 {
		t.Errorf("an app that answers was started again")
	}
}

func TestALiveAppThatWillNotStartAgainAfterARestartIsStopped(t *testing.T) {
	site, runner, _ := sharingSite(t)
	app := liveApp(t, site, "node a.js")
	runner.silent, runner.fail = errors.New("connection refused"), errors.New("no such file")
	site.server.ReconcileApps()
	site.server.Wait()
	if after, _ := site.server.apps.Get(app.ID); after.State != apps.Stopped || !strings.Contains(after.Note, "did not start again") {
		t.Errorf("an app that would not start again: %+v", after)
	}
}

func liveApp(t *testing.T, site *site, command string) apps.App {
	t.Helper()
	app, err := site.server.apps.New(apps.Sharer{Instance: "20260930-1200-notes", Owner: "ada@acme.com"}, command, 8080, 3)
	if err != nil {
		t.Fatal(err)
	}
	app, err = site.server.apps.Change(app.ID, func(a *apps.App) {
		a.State, a.Host, a.Target = apps.Live, "localhost:8080", apps.SocketTarget("/tmp/gone/app.sock")
	})
	if err != nil {
		t.Fatal(err)
	}
	return app
}
