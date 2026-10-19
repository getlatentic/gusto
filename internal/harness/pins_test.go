package harness

import (
	"os"
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
		pin, found := Pinned(adapter.Package)
		if !found {
			t.Errorf("%s: %s is not in pins.json; run internal/harness/pin.sh %s@<version>", name, adapter.Package, adapter.Package)
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

// The MCP servers the deployment's image carries are pinned the same way.
func TestEveryMCPServerTheImageCarriesIsPinned(t *testing.T) {
	text, err := os.ReadFile("../../deploy/ansible/group_vars/all.yml")
	if err != nil {
		t.Fatal(err)
	}
	block, _, _ := strings.Cut(strings.SplitN(string(text), "runner_mcp_packages:", 2)[1], "\n\n")
	for _, member := range regexp.MustCompile(`(?m)^\s+- "?([^"\s]+)"?\s*$`).FindAllStringSubmatch(block, -1) {
		if strings.Contains(member[1][1:], "@") {
			t.Errorf("%s carries a version; versions live in pins.json", member[1])
		}
		if _, found := Pinned(member[1]); !found {
			t.Errorf("%s is not in pins.json", member[1])
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
