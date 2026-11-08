package process

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/pkg/task"
)

// A harness run on a person's computer is given what a container gives it: a home of the task's own with each harness's
// configuration in it, and the instance's state as the place it keeps a conversation.
func TestATaskRunHereHasAHomeOfItsOwnAndTheInstancesStateAsItsData(t *testing.T) {
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	dir := task.Dir(t.TempDir())
	home, data, err := taskHome(backend.Turn{Dir: dir, Spec: task.Spec{Session: "a-conversation"}, State: instances.State("a-conversation")})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := filepath.Abs(instances.State("a-conversation"))
	if !filepath.IsAbs(home) || !strings.HasPrefix(home, string(dir)) || data != state {
		t.Errorf("home %s, data %s", home, data)
	}
	for _, file := range []string{"pi/gusto-provider.ts", "hermes/config.yaml", "opencode/opencode.json", "dsh/gusto.patch.yml"} {
		if _, err := os.Stat(filepath.Join(home, ".config", filepath.FromSlash(file))); err != nil {
			t.Errorf("the harness's configuration is not in its home: %v", err)
		}
	}
	if _, alone, _ := taskHome(backend.Turn{Dir: task.Dir(t.TempDir())}); !strings.HasSuffix(alone, filepath.Join(".local", "share")) {
		t.Errorf("a job of no conversation keeps its data in its own home, not %s", alone)
	}
}

func TestAHarnessRunHereReadsItsOwnHomeAndNotThePersons(t *testing.T) {
	env := homedEnv([]string{"PATH=/usr/bin", "HOME=/Users/ada", "XDG_CONFIG_HOME=/Users/ada/.config", "LANG=en_GB.UTF-8"}, "/jobs/1/home", "/state/a", "/cache/gusto")
	for _, want := range []string{"PATH=/usr/bin", "LANG=en_GB.UTF-8", "HOME=/jobs/1/home", "XDG_CONFIG_HOME=" + filepath.Join("/jobs/1/home", ".config"), "XDG_DATA_HOME=/state/a", "XDG_CACHE_HOME=/cache/gusto"} {
		if !slices.Contains(env, want) {
			t.Errorf("%s is not in %v", want, env)
		}
	}
	for _, entry := range env {
		if strings.Contains(entry, "/Users/ada") {
			t.Errorf("the person's own %s is still there", entry)
		}
	}
}

// A turn run in this process has no container to mount the instance into, so
// its working folder is a link. The task directory is made before it is queued, which
// leaves a real directory in the way — and a link cannot be put over one.
func TestATurnRunHereWorksInTheInstancesWorkingFolder(t *testing.T) {
	instance := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(instance, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "already.txt"), []byte("here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(instance, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir.WorkingFolder(), "already.txt")); err != nil {
		t.Fatalf("the turn does not work in the instance's workspace: %v", err)
	}
	// And doing it twice is doing it once, because a turn can be run again.
	if err := pointAt(instance, dir); err != nil {
		t.Fatalf("a second run could not point at it: %v", err)
	}
}

// gusto on a person's computer keeps its folders relative to its own, and a link's relative target is read from the link's
// folder, so the link has to name the working folder whole.
func TestATurnRunHereFindsTheWorkingFolderWhenTheFoldersAreRelative(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("sessions", "a", "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := task.Dir(filepath.Join("jobs", "1"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(filepath.Join("sessions", "a", "workspace"), dir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir.WorkingFolder()); err != nil || !info.IsDir() {
		t.Fatalf("the link goes nowhere: %v", err)
	}
}

// A task with a checkout of its own is not a turn, and must not lose it.
func TestAWorkingFolderWithSomethingInItIsNotReplacedByALink(t *testing.T) {
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir.WorkingFolder(), "mine.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(t.TempDir(), dir); err == nil {
		t.Fatal("a workspace with a file in it was replaced by a link")
	}
	if _, err := os.Stat(filepath.Join(dir.WorkingFolder(), "mine.txt")); err != nil {
		t.Errorf("the file was lost: %v", err)
	}
}

// door is a proxy that gives every turn the same token, or refuses every one.
type door struct {
	token   string
	refused error
}

func (d door) Token(task.Dir, task.Spec) (string, error) { return d.token, d.refused }

func (d door) SandboxToken(task.Dir, task.Spec) (string, bool, error) {
	return d.token, false, d.refused
}

func (d door) RecordRun(task.Dir, task.Spec, string, bool) error { return nil }

// A turn run in the server's own process goes through the proxy as a box does: it is given a token for its credential and the
// proxy on loopback, and none of the server's environment that is its own.
func TestATurnRunHereIsGivenTheProxyAndATokenAndNoneOfTheServersSecrets(t *testing.T) {
	t.Setenv("GUSTO_CREDENTIALS", t.TempDir())
	t.Setenv("GUSTO_PASSWORD", "the-servers-own-secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "an-aws-secret")
	turn := backend.Turn{Dir: task.Dir(t.TempDir()), Exit: backend.Exit{Door: door{token: "a-run-token"}, Port: "8041"}}
	env, err := agentEnv(turn)
	if err != nil {
		t.Fatal(err)
	}
	if env.ModelToken != "a-run-token" || env.ModelURL != "http://127.0.0.1:8041/model/v1" {
		t.Errorf("the turn was not given the proxy and its token: %+v", env)
	}
	joined := strings.Join(env.Base, "\n")
	for _, secret := range []string{"the-servers-own-secret", "an-aws-secret", "GUSTO_CREDENTIALS"} {
		if strings.Contains(joined, secret) {
			t.Errorf("%s is in the turn's environment", secret)
		}
	}
	for _, want := range []string{"PATH=", "HOME=", "GUSTO_MODEL_TOKEN=a-run-token"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the turn's environment lacks %s", want)
		}
	}
	turn.Exit.Door = door{refused: errors.New("no credential called nowhere")}
	if _, err := agentEnv(turn); err == nil {
		t.Error("a turn the proxy refused was given an environment")
	}
	direct := backend.Turn{Dir: turn.Dir, Exit: backend.Exit{Direct: "http://localhost:11434/v1"}}
	if env, err := agentEnv(direct); err != nil || env.ModelURL != "http://localhost:11434/v1" || env.ModelToken != "" {
		t.Errorf("a deployment with no proxy did not send the turn to its model directly: %+v (%v)", env, err)
	}
}
