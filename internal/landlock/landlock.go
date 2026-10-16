// Package landlock puts a process behind Linux's own sandbox, Landlock: the process can write only the folders it is given, can read
// only the system's folders and the ones it is given, and can connect to nothing on the network but gusto's proxy. It is a wall of
// the kernel's and not a virtual machine: the kernel is shared. It needs no program installed and no privilege. The network rules
// need a kernel of 6.7 or later; UDP, which is how a name is looked up, and a unix socket are not limited by any version used here.
package landlock

import (
	"encoding/json"
	"os"
	"os/exec"
)

// Policy is what one job may do.
type Policy struct {
	// Read are the folders the job may read and run programs from, beyond the system's own.
	Read []string `json:"read"`
	// Write are the folders the job may read, write and run programs from.
	Write []string `json:"write"`
	// ProxyPort is the one TCP port the job may connect to, which is gusto's proxy. Zero leaves the network open, as
	// GUSTO_JOB_NETWORK=open does.
	ProxyPort int `json:"proxyPort"`
}

// System are the folders every program needs to read to run, and nothing of a person's own. A person's home is not among them.
var System = []string{"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/etc", "/opt", "/proc", "/sys", "/run", "/var", "/srv", "/snap"}

// Scratch are the folders every program may write, where the system keeps what is not a person's.
var Scratch = []string{"/tmp", "/var/tmp", "/dev"}

// Save writes the policy where Wrap's command reads it.
func (p Policy) Save(path string) error {
	text, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, text, 0o600)
}

// Load reads a policy Save wrote.
func Load(path string) (Policy, error) {
	var p Policy
	text, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(text, &p)
}

// Wrap is the change to a command that runs it behind the policy in file, by the gusto program at self: `self sandbox-run FILE -- command`.
func Wrap(self, file string) func(*exec.Cmd) {
	return func(command *exec.Cmd) {
		command.Args = append([]string{self, "sandbox-run", file, "--"}, command.Args...)
		command.Path = self
	}
}
