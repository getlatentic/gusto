package harness

import (
	"regexp"
	"strings"
	"testing"
)

// The image installs what pins.json names, so every package an adapter names is pinned, and the pin is a whole one.
func TestEveryAdaptersPackageIsPinned(t *testing.T) {
	for _, name := range Names() {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.Package == "" {
			continue
		}
		if strings.Contains(adapter.Package[1:], "@") {
			t.Errorf("%s: the adapter names %q with a version; versions live in pins.json", name, adapter.Package)
		}
		if binary, found := PinnedBinary(adapter.Package); found {
			checkBinaryPin(t, name, binary)
			if adapter.Download == "" || adapter.Install() != "" {
				t.Errorf("%s: a program that is not on npm needs a download page, and has Install %q", name, adapter.Install())
			}
			continue
		}
		pin, found := Pinned(adapter.Package)
		if !found {
			t.Errorf("%s: %s is not in pins.json; run pkg/harness/pin.sh %s@<version>", name, adapter.Package, adapter.Package)
			continue
		}
		if pin.Version == "" || !strings.HasPrefix(pin.Integrity, "sha512-") || !strings.HasPrefix(pin.Tarball, "https://registry.npmjs.org/") {
			t.Errorf("%s: %s is pinned as %+v", name, adapter.Package, pin)
		}
		if adapter.Install() != adapter.Package+"@"+pin.Version {
			t.Errorf("%s: Install is %q", name, adapter.Install())
		}
	}
}

// A binary is pinned for both architectures the image is built for, each an https file with a whole sha256.
func checkBinaryPin(t *testing.T, name string, pin BinaryPin) {
	t.Helper()
	if pin.Version == "" {
		t.Errorf("%s: the binary pin has no version", name)
	}
	for _, arch := range []string{"x64", "arm64"} {
		artifact, found := pin.Artifacts[arch]
		if !found || !strings.HasPrefix(artifact.URL, "https://") || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(artifact.SHA256) {
			t.Errorf("%s: the %s build is pinned as %+v", name, arch, artifact)
		}
	}
}

// The MCP servers the runner image carries are pinned the same way.
func TestEveryMCPServerTheImageCarriesIsPinned(t *testing.T) {
	for _, pkg := range runnerList(t, "mcp.txt") {
		if strings.Contains(pkg[1:], "@") {
			t.Errorf("%s carries a version; versions live in pins.json", pkg)
		}
		if _, found := Pinned(pkg); !found {
			t.Errorf("%s is not in pins.json", pkg)
		}
	}
}

func TestTheBuildsAnImageRecordedAreReadByAgentName(t *testing.T) {
	versions := ReadVersions(strings.NewReader("opencode=opencode-ai@1.19.3\ncodex=@openai/codex@0.154.0\n\nnonsense\n"))
	if versions["opencode"] != "opencode-ai@1.19.3" || versions["codex"] != "@openai/codex@0.154.0" || len(versions) != 2 {
		t.Errorf("got %v", versions)
	}
	if VersionOf("opencode") != "" && !strings.HasPrefix(VersionOf("opencode"), "opencode-ai@") {
		t.Errorf("this computer records %q for opencode", VersionOf("opencode"))
	}
}
