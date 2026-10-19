package main

import (
	"crypto/rand"
	"encoding/hex"
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
	"strconv"
	"time"

	"github.com/getlatentic/gusto/internal/web"
	"strings"
)

// localHome is where gusto keeps a person's work on their own computer: GUSTO_LOCAL_HOME, or the system's place for an app's data.
func localHome() (string, error) {
	if home := os.Getenv("GUSTO_LOCAL_HOME"); home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		// The folder of its own in gusto's, as on Windows: gusto-helper keeps its settings in gusto's folder itself.
		return filepath.Join(userHome, "Library", "Application Support", "gusto", "data"), nil
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "gusto", "data"), nil
	}
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return filepath.Join(data, "gusto"), nil
	}
	return filepath.Join(userHome, ".local", "share", "gusto"), nil
}

// localCommand is gusto for one person on their own computer. The install script runs `gusto local install`, which makes gusto a
// service that starts at login and opens it; `gusto local run` is what that service runs; `gusto local stop` and
// `gusto local uninstall` take it away.
func localCommand(args []string) int {
	if len(args) == 0 || localCommands[args[0]] == nil {
		fmt.Fprintln(os.Stderr, localUsage)
		return 2
	}
	home, err := localHome()
	if err != nil {
		return fail(err)
	}
	return localCommands[args[0]](home, args[1:])
}

const localUsage = "usage: gusto local install|run|stop|uninstall|link, or gusto local use NAME [model...], or gusto local wall install"

// signInLink waits up to wait for gusto to answer, and is a new link that signs in to it, once and within five minutes.
func signInLink(setup localSetup, wait time.Duration) (string, error) {
	if !answers(setup.root()+"/healthz", wait) {
		return "", fmt.Errorf("gusto does not answer at %s: start it with gusto local install", setup.root())
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return setup.root() + web.LocalLink(setup.Secret, hex.EncodeToString(nonce), time.Now()), nil
}

// printLink prints a new sign-in link and opens nothing, for a program that shows gusto itself, such as the desktop app.
func printLink(home string, _ []string) int {
	setup, err := ensureLocal(home)
	if err != nil {
		return fail(err)
	}
	link, err := signInLink(setup, 2*time.Second)
	if err != nil {
		return fail(err)
	}
	fmt.Println(link)
	return 0
}

var localCommands = map[string]func(home string, args []string) int{
	"run":       func(home string, _ []string) int { return runLocal(home) },
	"install":   func(home string, _ []string) int { return installLocal(home) },
	"stop":      func(string, []string) int { return failIf(stopService()) },
	"uninstall": func(home string, _ []string) int { return uninstallLocal(home) },
	"use":       useKey,
	"link":      printLink,
	"wall":      localWall,
}

// installLocal makes gusto a service of this person's and opens it. It runs in the person's own shell, so it keeps the PATH
// that shell has, and says which model keys the shell has, without reading them.
func installLocal(home string) int {
	setup, err := ensureLocal(home)
	if err != nil {
		return fail(err)
	}
	setup.Path = os.Getenv("PATH")
	setup.FoundKeys = foundKeys()
	if err := setup.save(home); err != nil {
		return fail(err)
	}
	if err := installService(home); err != nil {
		return fail(err)
	}
	for _, need := range localNeeds() {
		fmt.Println(need)
	}
	for _, name := range setup.FoundKeys {
		fmt.Printf("Your shell has %s. To use it in gusto: gusto local use %s\n", name, name)
	}
	return openLocal(home, 20*time.Second)
}

func uninstallLocal(home string) int {
	if err := uninstallService(); err != nil {
		return fail(err)
	}
	if err := os.Remove(filepath.Join(home, "gusto.pid")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fail(err)
	}
	fmt.Println("gusto no longer starts. Its data stays in " + home + ": remove that folder to delete it.")
	return 0
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
	if err := keepLog(home); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(home, "gusto.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fail(err)
	}
	os.Setenv("GUSTO_PASSWORD", setup.Secret)
	os.Setenv("GUSTO_ADDR", fmt.Sprintf("127.0.0.1:%d", setup.Port))
	os.Setenv("GUSTO_PROXY_ADDR", fmt.Sprintf("127.0.0.1:%d", setup.ProxyPort))
	os.Setenv("GUSTO_LOCAL", "1")
	// The person adds the models they use, as the admin of their own registry, and a model on this computer is theirs to use.
	os.Setenv("GUSTO_REGISTRY", filepath.Join(home, "registry"))
	os.Setenv("GUSTO_REGISTRY_PRIVATE", "allow")
	// Started by the desktop app before anything recorded the person's PATH, gusto asks their shell for it.
	if setup.Path == "" {
		if setup.Path = loginShellPath(); setup.Path != "" {
			if err := setup.save(home); err != nil {
				return fail(err)
			}
		}
	}
	if setup.Path != "" {
		os.Setenv("PATH", setup.Path)
	}
	os.Setenv("GUSTO_FOUND_KEYS", strings.Join(setup.FoundKeys, ","))
	useInstalledWall(home)
	chooseWall()
	return serve(nil)
}

// openCommand opens gusto on this computer in the browser, signed in.
func openCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: gusto open")
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
	// gusto open is run in a terminal of the person's, so gusto looks for engines where that terminal does, which is where
	// they just installed one.
	if path := os.Getenv("PATH"); path != "" && path != setup.Path {
		setup.Path = path
		if err := setup.save(home); err != nil {
			return fail(err)
		}
	}
	link, err := signInLink(setup, wait)
	if err != nil {
		return fail(err)
	}
	fmt.Println("gusto is at " + setup.root())
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

// browseWait is how long a browser has to say it could not open a link. xdg-open runs as long as the browser it starts,
// so one still running then has opened it.
var browseWait = 3 * time.Second

// browse opens a link in the person's browser, and leaves the browser running when gusto ends.
func browse(link string) error {
	opener := exec.Command("xdg-open", link)
	switch runtime.GOOS {
	case "darwin":
		opener = exec.Command("open", link)
	case "windows":
		opener = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	}
	if err := opener.Start(); err != nil {
		return err
	}
	ended := make(chan error, 1)
	go func() { ended <- opener.Wait() }()
	select {
	case err := <-ended:
		return err
	case <-time.After(browseWait):
		return nil
	}
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
