package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/getlatentic/haas/internal/web"
)

// localSetup is what haas keeps for the one person it runs for on their own computer: the port it listens on, on the loopback,
// and the secret that is both its password and the key its sign-in links are made with. Only this person can read it.
type localSetup struct {
	Port   int    `json:"port"`
	Secret string `json:"secret"`
}

// localPort is the port haas on a person's computer listens on when it is free the first time.
const localPort = 7140

// localHome is where haas keeps a person's work on their own computer: HAAS_LOCAL_HOME, or the system's place for an app's data.
func localHome() (string, error) {
	if home := os.Getenv("HAAS_LOCAL_HOME"); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(userHome, "Library", "Application Support", "haas"), nil
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "haas", "data"), nil
	}
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return filepath.Join(data, "haas"), nil
	}
	return filepath.Join(userHome, ".local", "share", "haas"), nil
}

// ensureLocal reads the setup at home, or makes it the first time: a secret, and the usual port or a free one.
func ensureLocal(home string) (localSetup, error) {
	path := filepath.Join(home, "local.json")
	if text, err := os.ReadFile(path); err == nil {
		var setup localSetup
		if err := json.Unmarshal(text, &setup); err != nil || setup.Port == 0 || setup.Secret == "" {
			return localSetup{}, fmt.Errorf("%s is not a setup haas can read: remove it to make a new one", path)
		}
		return setup, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return localSetup{}, err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return localSetup{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return localSetup{}, err
	}
	setup := localSetup{Port: freePort(), Secret: hex.EncodeToString(secret)}
	text, err := json.MarshalIndent(setup, "", "  ")
	if err != nil {
		return localSetup{}, err
	}
	return setup, os.WriteFile(path, text, 0o600)
}

// freePort is the usual port when nothing listens on it, else one the system gives.
func freePort() int {
	for _, address := range []string{fmt.Sprintf("127.0.0.1:%d", localPort), "127.0.0.1:0"} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			continue
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		return port
	}
	return localPort
}

// localCommand is haas for one person on their own computer. The install script runs `haas local install`, which makes haas a
// service that starts at login and opens it; `haas local run` is what that service runs; `haas local stop` and
// `haas local uninstall` take it away.
func localCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: haas local install|run|stop|uninstall")
		return 2
	}
	home, err := localHome()
	if err != nil {
		return fail(err)
	}
	switch args[0] {
	case "run":
		return runLocal(home)
	case "install":
		if _, err := ensureLocal(home); err != nil {
			return fail(err)
		}
		if err := installService(home); err != nil {
			return fail(err)
		}
		return openLocal(home, 20*time.Second)
	case "stop":
		return failIf(stopService())
	case "uninstall":
		return failIf(uninstallService())
	}
	fmt.Fprintln(os.Stderr, "usage: haas local install|run|stop|uninstall")
	return 2
}

// runLocal is the server for one person: in their own folder, on the loopback, signed in by its own links, and with jobs in this
// process until the machine has a sandbox to give them (getlatentic/planning#653).
func runLocal(home string) int {
	setup, err := ensureLocal(home)
	if err != nil {
		return fail(err)
	}
	if err := os.Chdir(home); err != nil {
		return fail(err)
	}
	os.Setenv("HAAS_PASSWORD", setup.Secret)
	os.Setenv("HAAS_ADDR", fmt.Sprintf("127.0.0.1:%d", setup.Port))
	os.Setenv("HAAS_LOCAL", "1")
	if os.Getenv("HAAS_SANDBOX") == "" {
		os.Setenv("HAAS_SANDBOX", "here")
	}
	return serve(nil)
}

// openCommand opens haas on this computer in the browser, signed in.
func openCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: haas open")
		return 2
	}
	home, err := localHome()
	if err != nil {
		return fail(err)
	}
	return openLocal(home, 5*time.Second)
}

// openLocal waits up to wait for the server to answer, then opens a new sign-in link to it and prints it.
func openLocal(home string, wait time.Duration) int {
	setup, err := ensureLocal(home)
	if err != nil {
		return fail(err)
	}
	root := fmt.Sprintf("http://127.0.0.1:%d", setup.Port)
	if !answers(root+"/healthz", wait) {
		return fail(fmt.Errorf("haas does not answer at %s: start it with haas local install", root))
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return fail(err)
	}
	link := root + web.LocalLink(setup.Secret, hex.EncodeToString(nonce), time.Now())
	fmt.Println("haas is at " + root)
	if err := browse(link); err != nil {
		fmt.Println("Open this link to sign in, within five minutes: " + link)
	}
	return 0
}

func answers(url string, wait time.Duration) bool {
	client := http.Client{Timeout: time.Second}
	for deadline := time.Now().Add(wait); ; {
		if answer, err := client.Get(url); err == nil {
			answer.Body.Close()
			if answer.StatusCode == http.StatusOK {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// browse opens a link in the person's browser.
func browse(link string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", link).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link).Run()
	}
	return exec.Command("xdg-open", link).Run()
}

// localPerson is who the person on this computer is, for the record of what they asked.
func localPerson() string {
	if me, err := user.Current(); err == nil && me.Username != "" {
		return me.Username
	}
	return "you"
}

func failIf(err error) int {
	if err != nil {
		return fail(err)
	}
	return 0
}

// onLoopback is whether an address listens on this machine's loopback alone.
func onLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
