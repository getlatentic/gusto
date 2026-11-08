package harness

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
)

// pinsFile is pins.json: every npm package the runner image installs, at one version, with the registry's sha512 integrity
// of that tarball. pin.sh writes it. The image's build refuses a package whose bytes differ, so the agent a task runs is the
// one these pins name and not whatever the registry served that day.
//
//go:embed pins.json
var pinsFile []byte

// Pin is one package's version and the integrity of its tarball.
type Pin struct {
	Version   string `json:"version"`
	Integrity string `json:"integrity"`
	Tarball   string `json:"tarball"`
}

// BinaryPin is a program its maker publishes as one file for each architecture rather than on npm: its version, and for
// each architecture, by Node's name for it, where the file is and its sha256.
type BinaryPin struct {
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`
}

// Artifact is one architecture's file of a BinaryPin.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type pinFile struct {
	Packages map[string]Pin       `json:"packages"`
	Binaries map[string]BinaryPin `json:"binaries"`
}

var pinned = sync.OnceValue(func() pinFile {
	var file pinFile
	if err := json.Unmarshal(pinsFile, &file); err != nil {
		panic("pins.json: " + err.Error())
	}
	return file
})

// Pinned is the version the image installs of an npm package, when the package is pinned.
func Pinned(pkg string) (Pin, bool) {
	pin, found := pinned().Packages[pkg]
	return pin, found
}

// Pins is every pinned npm package, by name.
func Pins() map[string]Pin { return pinned().Packages }

// PinnedBinary is the version and files the image installs of a program that is not on npm, when it is pinned.
func PinnedBinary(name string) (BinaryPin, bool) {
	pin, found := pinned().Binaries[name]
	return pin, found
}

// Install is the package a person installs to run this agent as the image does: the package at its pinned version, or
// the bare package for one that is not pinned. It is empty for a program that is not on npm, which a person installs
// from the adapter's Download page.
func (a Adapter) Install() string {
	if _, binary := PinnedBinary(a.Package); binary {
		return ""
	}
	if pin, found := Pinned(a.Package); found {
		return a.Package + "@" + pin.Version
	}
	return a.Package
}

// BuildsFile is the file the runner image writes as it installs its agents: one line per agent, name=package@version, as
// the build resolved them. It is what the record names as the agent's build, and what a catalog reads for the version.
const BuildsFile = "/etc/gusto/harnesses"

// ReadVersions reads such a file: each agent's name to its package@version.
func ReadVersions(r io.Reader) map[string]string {
	versions := map[string]string{}
	lines := bufio.NewScanner(r)
	for lines.Scan() {
		if name, build, found := strings.Cut(strings.TrimSpace(lines.Text()), "="); found && name != "" && build != "" {
			versions[name] = build
		}
	}
	return versions
}

// VersionOf is the build of the named agent this image carries, or nothing where there is no such file: on a person's own
// computer, where nothing was installed by the image.
func VersionOf(name string) string {
	file, err := os.Open(BuildsFile)
	if err != nil {
		return ""
	}
	defer file.Close()
	return ReadVersions(file)[name]
}
