package sandbox

import (
	"slices"
	"strings"
	"testing"
)

// Whether the runtime holds to it is measured on a Linux machine. What can be held here is that the command asks for no
// network and no way out, gives the app only its own folder, home and socket, and holds it as a turn is held.
func TestAnAppsContainerHasNoNetworkAndNoWayOutButItsOwnSocket(t *testing.T) {
	args, err := plain().appArgv(AppBox{
		Name: "gusto-app-acme-0a1b2c3d4e", Owner: Owner{Org: "acme", Person: "p-1", Instance: "notes"}, App: "0a1b2c3d4e",
		Folder: "/srv/agents/notes/workspace", Home: "/srv/computers/p-1/home", SocketDir: "/tmp/gusto-apps-acme/0a1b2c3d4e",
		Command: "node server.js", Port: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--network=none", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory=3g",
		"/tmp/gusto-apps-acme/0a1b2c3d4e:" + AppSocketDir,
		"/srv/agents/notes/workspace:/job/workspace",
		"/srv/computers/p-1/home:" + Home, KeptHomeEnv + "=1",
		LabelPrefix + "org=acme", LabelPrefix + "person=p-1", LabelPrefix + "instance=notes", LabelPrefix + "app=0a1b2c3d4e",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("want %q in %v", want, args)
		}
	}
	joined := strings.Join(args, " ")
	for _, never := range []string{"--rm", InsideSocket, "GUSTO_MODEL_TOKEN", "PROXY", "--add-host", LabelPrefix + "job="} {
		if strings.Contains(joined, never) {
			t.Errorf("an app's container was given %q: %v", never, args)
		}
	}
	tail := args[slices.Index(args, "gusto:dev"):]
	if want := []string{"gusto:dev", "inside-app", "--socket", AppSocket, "--port", "3000", "--dir", "/job/workspace", "--", "node server.js"}; !slices.Equal(tail, want) {
		t.Errorf("the app is started by gusto inside-app: %v", tail)
	}
}

func TestAnAppWithNoKeptHomeIsGivenNone(t *testing.T) {
	args, err := plain().appArgv(AppBox{Name: "n", App: "a", Folder: "/f", SocketDir: "/s", Command: "c", Port: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, KeptHomeEnv+"=1") || strings.Contains(strings.Join(args, " "), ":"+Home) {
		t.Errorf("%v", args)
	}
}
