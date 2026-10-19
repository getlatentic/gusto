package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
)

// localSetup is what gusto keeps for the one person it runs for on their own computer. Only this person can read it.
type localSetup struct {
	// Port is where the server listens, and ProxyPort where its proxy does, both on the loopback.
	Port      int `json:"port"`
	ProxyPort int `json:"proxyPort"`
	// Secret is both the server's password and the key its sign-in links are made with.
	Secret string `json:"secret"`
	// Path is the person's PATH as their shell had it when gusto was installed. A service manager starts gusto with a bare one,
	// and the harnesses a person installs are where their own shell finds them.
	Path string `json:"path,omitempty"`
	// FoundKeys is the model keys the person's shell had when gusto was installed, by name alone: gusto offers them and does
	// not take them.
	FoundKeys []string `json:"foundKeys,omitempty"`
}

// adoptRecordedPath has the running server look for programs on the PATH the person last ran gusto from, which the setup in
// the folder the server runs in records. It is read each time engines are looked for, since a person installs one into a
// folder the PATH they installed gusto with did not name.
func adoptRecordedPath() {
	text, err := os.ReadFile("local.json")
	if err != nil {
		return
	}
	var setup localSetup
	if json.Unmarshal(text, &setup) == nil && setup.Path != "" && setup.Path != os.Getenv("PATH") {
		os.Setenv("PATH", setup.Path)
	}
}

// The ports gusto on a person's computer listens on when they are free the first time.
const (
	localPort      = 7140
	localProxyPort = 7141
)

// ensureLocal reads the setup at home, or makes it the first time: a secret, and the usual ports or free ones. A setup made
// before gusto had a proxy of its own is given a port for one.
func ensureLocal(home string) (localSetup, error) {
	path := filepath.Join(home, "local.json")
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return newLocal(home)
	}
	if err != nil {
		return localSetup{}, err
	}
	var setup localSetup
	if err := json.Unmarshal(text, &setup); err != nil || setup.Port == 0 || setup.Secret == "" {
		return localSetup{}, fmt.Errorf("%s is not a setup gusto can read: remove it to make a new one", path)
	}
	if setup.ProxyPort == 0 {
		setup.ProxyPort = freePort(localProxyPort)
		return setup, setup.save(home)
	}
	return setup, nil
}

func newLocal(home string) (localSetup, error) {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return localSetup{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return localSetup{}, err
	}
	setup := localSetup{Port: freePort(localPort), Secret: hex.EncodeToString(secret)}
	setup.ProxyPort = freePort(localProxyPort)
	if setup.ProxyPort == setup.Port {
		setup.ProxyPort = freePort(0)
	}
	return setup, setup.save(home)
}

// root is where gusto on this computer answers.
func (s localSetup) root() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

func (s localSetup) save(home string) error {
	text, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(home, "local.json"), text, 0o600)
}

// freePort is the port asked for when nothing listens on it, else one the system gives.
func freePort(preferred int) int {
	for _, address := range []string{fmt.Sprintf("127.0.0.1:%d", preferred), "127.0.0.1:0"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		return port
	}
	return preferred
}
