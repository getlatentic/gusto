//go:build unix

package runner

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAppHelper is the app ServeApp starts in these tests: this test binary, serving on the PORT it is given, until it is
// stopped. It does nothing when it is run as a test.
func TestAppHelper(t *testing.T) {
	if os.Getenv("GUSTO_APP_HELPER") != "1" {
		return
	}
	cwd, _ := os.Getwd()
	http.ListenAndServe(os.Getenv("HOST")+":"+os.Getenv("PORT"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello from " + filepath.Base(cwd) + r.URL.Path))
	}))
}

func TestAnAppIsReachedThroughItsSocketAndRunsInTheAgentsFolderUntilItIsStopped(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gusto-serve-app")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	folder := filepath.Join(dir, "workspace")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUSTO_APP_HELPER", "1")
	app := App{
		Command: "exec '" + os.Args[0] + "' -test.run '^TestAppHelper$'",
		Port:    freePort(t), Folder: folder, Socket: filepath.Join(dir, "app.sock"),
	}
	ctx, stop := context.WithCancel(context.Background())
	ended := make(chan error, 1)
	go func() {
		_, err := ServeApp(ctx, app, Place{})
		ended <- err
	}()

	said := askThrough(t, app.Socket, "/notes")
	if said != "hello from workspace/notes" {
		t.Fatalf("the app said %q", said)
	}
	if info, err := os.Stat(app.Socket); err != nil || info.Mode().Perm() != 0o666 {
		t.Errorf("the server outside, which may be another user, can connect: %v %v", info.Mode(), err)
	}
	stop()
	select {
	case err := <-ended:
		if err != nil {
			t.Errorf("stopping the app: %v", err)
		}
	case <-time.After(appStop + 5*time.Second):
		t.Fatal("the app did not stop")
	}
}

// askThrough asks the app on the socket until it answers: it takes a moment to listen.
func askThrough(t *testing.T, socket, path string) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := client.Get("http://localhost" + path)
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return strings.TrimSpace(string(body))
		}
		if time.Now().After(deadline) {
			t.Fatalf("the app never answered: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
