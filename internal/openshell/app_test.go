package openshell_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/runner"
)

var appConfig = openshell.Config{Binary: "openshell"}

// appSandbox is a fake computer with the directory an agent's turns work in.
func appSandbox(t *testing.T, root, instance string) string {
	t.Helper()
	const name = "c-11111111-22222222"
	if err := os.MkdirAll(filepath.Join(root, name)+openshell.InstanceDir(instance), 0o755); err != nil {
		t.Fatal(err)
	}
	return name
}

func waitFor(t *testing.T, what string, ready func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if ready() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s did not happen", what)
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestAnAppIsStartedInTheAgentsDirectoryWithItsPortAndStoppedWithItsProcesses(t *testing.T) {
	root := installFake(t)
	sandbox := appSandbox(t, root, "20260930-1200-notes")
	pidFile := filepath.Join(root, sandbox, "sandbox", "apps", "0123456789.pid")
	logFile := filepath.Join(root, sandbox, "sandbox", "apps", "0123456789.log")
	t.Cleanup(func() {
		if text, err := os.ReadFile(pidFile); err == nil {
			if pid, _ := strconv.Atoi(strings.TrimSpace(string(text))); pid > 0 {
				syscall.Kill(-pid, syscall.SIGKILL)
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	ctx := context.Background()

	if err := openshell.StartApp(ctx, appConfig, sandbox, "20260930-1200-notes", "0123456789", `echo "port=$PORT host=$HOST in $(basename "$(dirname "$PWD")")/$(basename "$PWD")"; exec sleep 60`, 8080); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the app writing its log", func() bool {
		text, _ := os.ReadFile(logFile)
		return strings.Contains(string(text), "port=8080 host=0.0.0.0 in ")
	})
	if got := openshell.AppLog(ctx, appConfig, sandbox, "0123456789"); !strings.Contains(got, "in instance-20260930-1200-notes/workspace") {
		t.Errorf("it works in the agent's own directory: %q", got)
	}
	text, _ := os.ReadFile(pidFile)
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil || !alive(pid) {
		t.Fatalf("the app is running: %q %v", text, err)
	}

	if err := openshell.StopApp(ctx, appConfig, sandbox, "0123456789"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the app stopping", func() bool { return !alive(pid) })
	if _, err := os.Stat(pidFile); err == nil {
		t.Error("the pid file was left")
	}
	if err := openshell.StopApp(ctx, appConfig, sandbox, "0123456789"); err != nil {
		t.Errorf("an app that is not running is stopped: %v", err)
	}
}

func TestExposingAnAppGivesTheHostTheGatewayAnswersToAndWhereToAskIt(t *testing.T) {
	root := installFake(t)
	t.Setenv("OPENSHELL_FAKE_GATEWAY_PORT", "18999")
	sandbox := appSandbox(t, root, "20260930-1200-notes")
	address, err := openshell.ExposeApp(context.Background(), appConfig, sandbox, 8080, "a0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if address.Host != "default--"+sandbox+"--a0123456789.openshell.localhost:18999" || address.Target != "127.0.0.1:18999" {
		t.Errorf("%+v", address)
	}
	if err := openshell.UnexposeApp(context.Background(), appConfig, sandbox, "a0123456789"); err != nil {
		t.Error(err)
	}
	if err := openshell.UnexposeApp(context.Background(), appConfig, sandbox, "a0123456789"); err != nil {
		t.Errorf("a service that is not there is taken away: %v", err)
	}
	if calls, _ := os.ReadFile(filepath.Join(root, "calls.log")); !strings.Contains(string(calls), "service expose "+sandbox+" 8080 a0123456789") {
		t.Errorf("the gateway was asked for the port and the name:\n%s", calls)
	}
}

func TestAnAppTheAgentNeverWroteALogForHasNothingToSay(t *testing.T) {
	root := installFake(t)
	sandbox := appSandbox(t, root, "20260930-1200-notes")
	if got := openshell.AppLog(context.Background(), appConfig, sandbox, "0123456789"); got != "" {
		t.Errorf("%q", got)
	}
}

// An agent is told how the sandbox's network behaves, whatever set-up it has or none, because the
// first app it writes fails on it otherwise: Node's listen(port) resets IPv4 clients here.
func TestEveryAgentIsGivenTheGuidanceOnTheSandboxsNetworkWhetherOrNotItHasASetup(t *testing.T) {
	root := installFake(t)
	dir, _ := aJob(t)
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), `{"name":"fixture","program":"sh","skills":"/root/.config/fixture/skills",
	  "argv":["-c","printf yes > answer.txt; cat ../../../.config/fixture/skills/haas-sandbox/SKILL.md"],"needs":"nothing","stream":"plain"}`)
	t.Setenv("HAAS_HARNESSES", harnesses)
	if exit, err := runner.Outside(dir, inComputer(t, dir, "c-12345678-abcdefgh", "20260930-1200-guidance")); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	for _, want := range []string{"name: haas-sandbox", "PORT", "0.0.0.0", "listen(port, '0.0.0.0')", "does not resolve", "relative", "no network but the model"} {
		if !strings.Contains(string(said), want) {
			t.Errorf("the guidance does not say %q:\n%s", want, said)
		}
	}
	_ = root
}
