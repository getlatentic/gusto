//go:build linux

package container

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// An app is started in a real container, answers through its socket, reaches nothing, and is gone once stopped. It needs a
// runtime and a runner image, named by GUSTO_APP_CHECK_IMAGE, that carries node, so it runs only where one is named.
func TestAnAppRunsInARealContainerThatReachesNothing(t *testing.T) {
	image := os.Getenv("GUSTO_APP_CHECK_IMAGE")
	if image == "" {
		t.Skip("GUSTO_APP_CHECK_IMAGE names no runner image to start an app from")
	}
	folders := t.TempDir()
	folder := filepath.Join(folders, "notes", "workspace")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	server := `require("http").createServer((q, r) => require("https").get("https://example.com", () => r.end("reached")).on("error", e => r.end("alone " + e.code))).listen(process.env.PORT, process.env.HOST)`
	if err := os.WriteFile(filepath.Join(folder, "server.js"), []byte(server), 0o644); err != nil {
		t.Fatal(err)
	}
	b := New(Config{Org: "check", Containers: sandbox.Containers{Runtime: "docker", Image: image, CPUs: "1", Memory: "1g"},
		AgentFolder: func(instance string) string { return filepath.Join(folders, instance, "workspace") }})
	runner := b.Apps()
	if runner == nil {
		t.Fatal("no apps on Linux with agents' folders")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	app, err := runner.Start(ctx, apps.App{ID: "0a1b2c3d4e", Instance: "notes", Command: "node server.js", Port: 3000})
	if err != nil {
		t.Fatal(err)
	}
	socket, _ := app.Socket()
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	response, err := client.Get("http://" + app.Dialed() + "/")
	if err != nil {
		t.Fatal(err)
	}
	said, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.HasPrefix(string(said), "alone ") {
		t.Errorf("the app's container reached out: %s", said)
	}
	if err := runner.Stop(ctx, app); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("docker", "ps", "-aq", "--filter", "name=gusto-app-check-0a1b2c3d4e").Output(); len(out) > 0 {
		t.Errorf("the app's container is left: %s", out)
	}
}
