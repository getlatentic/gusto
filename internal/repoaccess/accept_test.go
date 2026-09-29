package repoaccess_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/repoaccess"
)

func TestOnlyAnAddressGitReachesOverAnEncryptedTransportIsAccepted(t *testing.T) {
	roots := []string{"/srv/repos"}
	for remote, ok := range map[string]bool{
		"https://github.com/acme/api":              true,
		"https://github.com/acme/api.git":          true,
		"ssh://git@github.com/acme/api.git":        true,
		"git@github.com:acme/api.git":              true,
		"/srv/repos/sample":                        true,
		"/srv/repos":                               true,
		"/srv/repos/../secrets":                    false,
		"/srv/repos-other/x":                       false,
		"/etc":                                     false,
		"/var/lib/haas/sessions/agent-1/workspace": false,
		"relative/path":                            false,
		"../elsewhere":                             false,
		"file:///etc":                              false,
		"http://github.com/acme/api":               false,
		"git://github.com/acme/api":                false,
		"ext::sh -c 'touch /tmp/x'":                false,
		"fd::3":                                    false,
		"--upload-pack=touch /tmp/x;":              false,
		"-c core.sshCommand=touch":                 false,
		"":                                         false,
		"   ":                                      false,
		"https://github.com/acme/api\nhttps://x/y": false,
	} {
		err := repoaccess.Acceptable(remote, roots)
		if (err == nil) != ok {
			t.Errorf("%q: accepted=%v, want %v (%v)", remote, err == nil, ok, err)
		}
	}
	if err := repoaccess.Acceptable("/srv/repos/sample", nil); err == nil {
		t.Error("a local path was accepted with no directory named")
	}
}

func TestARefThatGitWouldReadAsAnOptionIsRefused(t *testing.T) {
	for ref, ok := range map[string]bool{"main": true, "HEAD": true, "v1.2.3": true, "3ac3db3": true, "feature/x": true, "-b": false, "--force": false, "a\nb": false} {
		if (repoaccess.AcceptableRef(ref) == nil) != ok {
			t.Errorf("%q accepted=%v, want %v", ref, !ok, ok)
		}
	}
}

// Whatever reaches the clone, git is told the address is not an option, and may
// not run a program to reach it.
func TestACloneNeverReadsTheAddressAsAnOption(t *testing.T) {
	work := t.TempDir()
	marker := filepath.Join(work, "ran")
	forge := repoaccess.Forge{Web: "https://github.com"}
	for _, remote := range []string{"--upload-pack=touch " + marker + ";", "ext::sh -c 'touch " + marker + "'"} {
		_ = forge.Clone(context.Background(), remote, work, "into")
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("cloning %q ran a program", remote)
		}
	}
	if output, err := exec.Command("git", "--version").CombinedOutput(); err != nil || !strings.Contains(string(output), "git version") {
		t.Skip("no git")
	}
}
