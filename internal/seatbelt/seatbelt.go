// Package seatbelt puts a process behind macOS's own sandbox, sandbox-exec: the process can write only the folders it is given,
// cannot read the person's home but for what its engine is made of, and can reach nothing on the network but gusto's proxy. It is a sandbox
// of the kernel's and not a virtual machine: the computer's kernel is shared, and what is allowed is what a profile says.
package seatbelt

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Command is the program that applies a profile.
const Command = "/usr/bin/sandbox-exec"

// Policy is what one task may do.
type Policy struct {
	// Home is the person's home, which is closed to reading but for Readable.
	Home string
	// Writable are the folders the task may write: its workspace, its own home and state, and where it caches what it downloads.
	Writable []string
	// Readable are the folders under Home the task may still read: its own, and the ones its engine is installed in.
	Readable []string
	// ProxyPort is the one port on this computer the task may connect to, which is gusto's proxy. Zero opens the network, as
	// GUSTO_JOB_NETWORK=open does.
	ProxyPort int
}

// Profile is the policy as the sandbox reads it. The last rule that matches decides, so each deny is followed by what it lets through.
func (p Policy) Profile() string {
	var out strings.Builder
	out.WriteString("(version 1)\n(allow default)\n")

	out.WriteString("(deny file-write*)\n(allow file-write*")
	for _, folder := range sorted(p.Writable) {
		fmt.Fprintf(&out, " (subpath %s)", quote(folder))
	}
	out.WriteString(` (subpath "/private/tmp") (subpath "/private/var/folders") (literal "/dev/null") (literal "/dev/dtracehelper") (regex #"^/dev/(tty|fd/)"))` + "\n")

	if p.Home != "" {
		fmt.Fprintf(&out, "(deny file-read-data (subpath %s))\n", quote(p.Home))
		if readable := append(append([]string{}, p.Writable...), p.Readable...); len(readable) > 0 {
			out.WriteString("(allow file-read-data")
			for _, folder := range sorted(readable) {
				fmt.Fprintf(&out, " (subpath %s)", quote(folder))
			}
			out.WriteString(")\n")
		}
	}

	if p.ProxyPort > 0 {
		out.WriteString("(deny network*)\n")
		out.WriteString(`(allow network-bind (local ip "localhost:*"))` + "\n")
		out.WriteString(`(allow network-inbound (local ip "localhost:*"))` + "\n")
		fmt.Fprintf(&out, "(allow network-outbound (remote ip \"localhost:%d\"))\n", p.ProxyPort)
		out.WriteString(`(allow network-outbound (remote unix-socket))` + "\n")
	}
	return out.String()
}

// Wrap is what runs a command behind the profile in the file: the same command, its arguments, environment, folder and streams.
func Wrap(profile string) func(*exec.Cmd) {
	return func(command *exec.Cmd) {
		command.Args = append([]string{Command, "-f", profile, "--", command.Path}, command.Args[1:]...)
		command.Path = Command
	}
}

// Available says whether this computer can put a process behind a profile, by doing it.
func Available() error {
	out, err := exec.Command(Command, "-p", "(version 1)(allow default)", "/usr/bin/true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox-exec does not run here: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Readable is the folders under home the engine whose program is given needs to read: where the program really is, once its links are
// followed, and the node_modules tree it is installed in.
func Readable(program, home string) []string {
	resolved, err := filepath.EvalSymlinks(program)
	if err != nil {
		return nil
	}
	folders := []string{filepath.Dir(resolved)}
	for folder := filepath.Dir(resolved); folder != "/" && folder != filepath.Dir(folder); folder = filepath.Dir(folder) {
		if filepath.Base(folder) == "node_modules" {
			folders = append(folders, folder)
			break
		}
	}
	var under []string
	for _, folder := range folders {
		if strings.HasPrefix(folder, home+string(filepath.Separator)) {
			under = append(under, folder)
		}
	}
	return under
}

func quote(path string) string { return strconv.Quote(path) }

func sorted(folders []string) []string {
	unique := map[string]bool{}
	for _, folder := range folders {
		if folder != "" {
			unique[filepath.Clean(folder)] = true
		}
	}
	out := make([]string, 0, len(unique))
	for folder := range unique {
		out = append(out, folder)
	}
	sort.Strings(out)
	return out
}
