package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/job"
)

// A harness run on a person's computer is given what a container gives it: a home of the job's own with each harness's
// configuration in it, and the instance's state as the place it keeps a conversation.
func TestAJobRunHereHasAHomeOfItsOwnAndTheInstancesStateAsItsData(t *testing.T) {
	instances, err := instance.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	home, data, err := jobHome(dir, instances, job.Spec{Session: "a-conversation"})
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
	if _, alone, _ := jobHome(job.Dir(t.TempDir()), instances, job.Spec{}); !strings.HasSuffix(alone, filepath.Join(".local", "share")) {
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
