package agentinstall

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/harness"
)

// fakeNpm is an npm that packs the tarball given and installs by writing the agent's program into the prefix's bin.
func fakeNpm(t *testing.T, tarball []byte) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake npm is a shell script")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "agent.tgz")
	if err := os.WriteFile(source, tarball, 0o644); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
pack) cp "` + source + `" "$4/agent-1.0.0.tgz"; echo '[{"filename":"agent-1.0.0.tgz"}]' ;;
install) mkdir -p "$4/bin"; printf '#!/bin/sh\n' > "$4/bin/claude-agent-acp"; chmod +x "$4/bin/claude-agent-acp"; echo "$@" > "$4/asked" ;;
esac
`
	npm := filepath.Join(dir, "npm")
	if err := os.WriteFile(npm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return npm
}

func pinOf(tarball []byte) func(string) (harness.Pin, bool) {
	sum := sha512.Sum512(tarball)
	return func(pkg string) (harness.Pin, bool) {
		return harness.Pin{Version: "1.0.0", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:])}, pkg == "@agentclientprotocol/claude-agent-acp"
	}
}

func TestAnAgentIsInstalledAtItsPinnedVersionIntoGustosOwnFolder(t *testing.T) {
	tarball := []byte("the pinned bytes")
	installer := &Installer{Dir: t.TempDir(), Npm: fakeNpm(t, tarball), Pins: pinOf(tarball)}
	got, err := installer.Install(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if got != "@agentclientprotocol/claude-agent-acp@1.0.0" {
		t.Errorf("installed %s", got)
	}
	if _, err := os.Stat(filepath.Join(installer.Bin(), "claude-agent-acp")); err != nil {
		t.Errorf("the program is not in gusto's folder: %v", err)
	}
	asked, _ := os.ReadFile(filepath.Join(installer.Dir, "asked"))
	if !strings.Contains(string(asked), "--global --prefix "+installer.Dir) {
		t.Errorf("npm was asked %q, not to install into gusto's folder", asked)
	}
	if path := installer.WithBin(installer.WithBin("/usr/bin")); path != installer.Bin()+string(os.PathListSeparator)+"/usr/bin" {
		t.Errorf("the PATH became %s", path)
	}
}

func TestATarballThatIsNotThePinnedOneIsRefusedAndNothingIsInstalled(t *testing.T) {
	installer := &Installer{Dir: t.TempDir(), Npm: fakeNpm(t, []byte("other bytes")), Pins: pinOf([]byte("the pinned bytes"))}
	_, err := installer.Install(context.Background(), "claude")
	if err == nil || !strings.Contains(err.Error(), "not the pinned one") {
		t.Fatalf("installing gave %v", err)
	}
	if _, err := os.Stat(installer.Bin()); !os.IsNotExist(err) {
		t.Error("something was installed")
	}
}

func TestOnlyAPinnedAgentThatDoesNotSignInAsAPersonIsInstallable(t *testing.T) {
	installer := &Installer{Dir: t.TempDir(), Pins: pinOf(nil)}
	if !installer.Installable("claude") || installer.Installable("codex") || installer.Installable("no-such-agent") {
		t.Error("what can be installed is wrong")
	}
	if _, err := (&Installer{Dir: t.TempDir(), Npm: filepath.Join(t.TempDir(), "absent")}).Install(context.Background(), "nope"); err == nil {
		t.Error("an agent gusto does not have was installed")
	}
}

// TestARealAgentInstalls installs Claude Code's agent from npm for real, at the pinned version, into the folder
// GUSTO_REAL_INSTALL names. It needs the network and npm, so it runs only when asked.
func TestARealAgentInstalls(t *testing.T) {
	dir := os.Getenv("GUSTO_REAL_INSTALL")
	if dir == "" {
		t.Skip("set GUSTO_REAL_INSTALL to a folder to install into")
	}
	installer := &Installer{Dir: dir}
	got, err := installer.Install(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installer.Bin(), "claude-agent-acp")); err != nil {
		t.Fatalf("installed %s, and its program is not in %s: %v", got, installer.Bin(), err)
	}
	t.Logf("installed %s into %s", got, installer.Bin())
}
