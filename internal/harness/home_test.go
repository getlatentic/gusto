package harness

import "testing"

// An adapter is written for a container, where the harness's home is /root, so every mention of it is moved to the home it
// has elsewhere, and nothing that only looks like it.
func TestTheContainersHomeIsRewrittenAndNothingThatLooksLikeIt(t *testing.T) {
	for from, want := range map[string]string{
		"/root": "/sandbox", "/root/.local/share/codex": "/sandbox/.local/share/codex",
		`mkdir -p /root/.a && cp /root/.config/x /root/.b`: `mkdir -p /sandbox/.a && cp /sandbox/.config/x /sandbox/.b`,
		"/rootless/thing": "/rootless/thing", "/usr/root/x": "/usr/root/x",
	} {
		if got := Rehome(from, "/sandbox"); got != want {
			t.Errorf("%q became %q, want %q", from, got, want)
		}
	}
}

func TestAnAdapterAtAnotherHomeNamesItAndTheOriginalIsUnchanged(t *testing.T) {
	adapter := Adapter{
		Argv: []string{"--config=/root/.x"}, Resume: []string{"--state", "/root/.local/share/x"},
		Env:   map[string]string{"X_HOME": "/root/.local/share/x", "KEY": "{modelToken}"},
		Keeps: "/root/.local/share/x", Skills: "/root/.x/skills",
	}
	moved := adapter.At("/home/ada/jobs/1/home", "/home/ada/instances/a/state")
	if moved.Argv[0] != "--config=/home/ada/jobs/1/home/.x" || moved.Resume[1] != "/home/ada/instances/a/state/x" ||
		moved.Env["X_HOME"] != "/home/ada/instances/a/state/x" || moved.Env["KEY"] != "{modelToken}" ||
		moved.Keeps != "/home/ada/instances/a/state/x" || moved.Skills != "/home/ada/jobs/1/home/.x/skills" {
		t.Errorf("%+v", moved)
	}
	if adapter.Argv[0] != "--config=/root/.x" || adapter.Env["X_HOME"] != "/root/.local/share/x" {
		t.Errorf("the original changed: %+v", adapter)
	}
}
