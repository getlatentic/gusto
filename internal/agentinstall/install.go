// Package agentinstall puts an agent on a person's own computer the way the runner image carries it: its npm package at the
// version pins.json names, refused unless the tarball's sha512 is the pinned one, installed into gusto's own folder rather than
// npm's global one, so the person's own setup is left as it was and removing gusto removes the agents with it.
package agentinstall

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/getlatentic/gusto/pkg/harness"
)

// ErrNoNode is a computer without npm, which every agent gusto installs comes through.
var ErrNoNode = errors.New("installing an agent needs Node.js and npm: get them from nodejs.org, then try again")

// Installer installs agents into Dir.
type Installer struct {
	Dir string
	// Npm is the npm to run, found on the PATH when it is empty.
	Npm string
	// Pins is the version and tarball integrity each package must have, pins.json's when it is nil.
	Pins func(pkg string) (harness.Pin, bool)
	one  sync.Mutex
}

func (i *Installer) pinned(pkg string) (harness.Pin, bool) {
	if i.Pins != nil {
		return i.Pins(pkg)
	}
	return harness.Pinned(pkg)
}

// Bin is the folder the installed agents' programs are in, which belongs on the PATH.
func (i *Installer) Bin() string {
	if runtime.GOOS == "windows" {
		return i.Dir
	}
	return filepath.Join(i.Dir, "bin")
}

// WithBin is a PATH with the installed agents' folder first, once.
func (i *Installer) WithBin(path string) string {
	bin := i.Bin()
	for _, each := range filepath.SplitList(path) {
		if each == bin {
			return path
		}
	}
	if path == "" {
		return bin
	}
	return bin + string(os.PathListSeparator) + path
}

// Installable says whether an agent can be installed this way: it comes as a pinned npm package, and it is not one that signs in
// as a person, which runs where that person is signed in.
func (i *Installer) Installable(name string) bool {
	adapter, err := harness.Load("", name)
	if err != nil {
		return false
	}
	_, pinned := i.pinned(adapter.Package)
	return adapter.Package != "" && pinned && adapter.Needs != harness.NeedsAccount
}

// Install installs the named agent, and says the package and version it installed. One install runs at a time.
func (i *Installer) Install(ctx context.Context, name string) (string, error) {
	adapter, err := harness.Load("", name)
	if err != nil {
		return "", fmt.Errorf("gusto has no agent called %s", name)
	}
	if !i.Installable(name) {
		return "", fmt.Errorf("%s cannot be installed by gusto", adapter.Title)
	}
	pin, _ := i.pinned(adapter.Package)
	npm := i.Npm
	if npm == "" {
		found, err := exec.LookPath("npm")
		if err != nil {
			return "", ErrNoNode
		}
		npm = found
	}
	i.one.Lock()
	defer i.one.Unlock()
	if err := os.MkdirAll(i.Dir, 0o755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(i.Dir, ".stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	wanted := adapter.Package + "@" + pin.Version
	tarball, err := pack(ctx, npm, stage, wanted)
	if err != nil {
		return "", err
	}
	if err := checkIntegrity(tarball, pin.Integrity); err != nil {
		return "", fmt.Errorf("%s: %w", wanted, err)
	}
	install := exec.CommandContext(ctx, npm, "install", "--global", "--prefix", i.Dir, "--no-fund", "--no-audit", tarball)
	install.Env = npmEnv(stage)
	if said, err := install.CombinedOutput(); err != nil {
		return "", fmt.Errorf("npm could not install %s: %s", wanted, lastLine(said))
	}
	return wanted, nil
}

// pack fetches the package's tarball into the stage and says where it is.
func pack(ctx context.Context, npm, stage, wanted string) (string, error) {
	fetch := exec.CommandContext(ctx, npm, "pack", wanted, "--pack-destination", stage, "--json")
	fetch.Env = npmEnv(stage)
	said, err := fetch.Output()
	if err != nil {
		var failed *exec.ExitError
		if errors.As(err, &failed) {
			return "", fmt.Errorf("npm could not fetch %s: %s", wanted, lastLine(failed.Stderr))
		}
		return "", fmt.Errorf("npm could not fetch %s: %w", wanted, err)
	}
	var packed []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(said, &packed); err != nil || len(packed) == 0 {
		return "", fmt.Errorf("npm did not say what it fetched for %s", wanted)
	}
	return filepath.Join(stage, filepath.Base(packed[0].Filename)), nil
}

func checkIntegrity(tarball, want string) error {
	bytes, err := os.ReadFile(tarball)
	if err != nil {
		return err
	}
	sum := sha512.Sum512(bytes)
	if got := "sha512-" + base64.StdEncoding.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("the tarball is not the pinned one: its sha512 is %s, and pins.json has %s", got, want)
	}
	return nil
}

// npmEnv keeps npm's cache in the stage, so it goes with it.
func npmEnv(stage string) []string {
	return append(os.Environ(), "npm_config_cache="+filepath.Join(stage, "cache"), "npm_config_update_notifier=false")
}

func lastLine(said []byte) string {
	lines := strings.Split(strings.TrimSpace(string(said)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
