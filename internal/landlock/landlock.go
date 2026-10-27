// Package landlock puts a process behind Linux's own sandbox, Landlock: the process can write only the folders it is given, can read
// only the system's folders and the ones it is given, and can connect to nothing on the network but gusto's proxy. It is a sandbox of
// the kernel's and not a virtual machine: the kernel is shared. It needs no program installed and no privilege. The network rules
// need a kernel of 6.7 or later; UDP, which is how a name is looked up, and a unix socket are not limited by any version used here.
package landlock

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// Policy is what one task may do.
type Policy struct {
	// Read are the folders the task may read and run programs from, beyond the system's own.
	Read []string `json:"read"`
	// Write are the folders the task may read, write and run programs from.
	Write []string `json:"write"`
	// ProxyPort is the one TCP port the task may connect to, which is gusto's proxy. Zero leaves the network open, as
	// GUSTO_JOB_NETWORK=open does.
	ProxyPort int `json:"proxyPort"`
	// Temp is the task's own temporary folder. When it is set, the system's shared ones are not writable, since other tasks
	// on the same computer leave what is theirs there.
	Temp string `json:"temp,omitempty"`
}

// scratch is the folders the task may write besides its own: the task's temporary folder and the devices, or the system's
// shared temporary folders when it has none.
func (p Policy) scratch() []string {
	if p.Temp != "" {
		return []string{p.Temp, "/dev"}
	}
	return Scratch
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

// Load reads a policy Save wrote, or one given whole as JSON, which a command started in a computer the task shares is given
// so that no file the task could change holds its own rules.
func Load(pathOrJSON string) (Policy, error) {
	var p Policy
	text := []byte(pathOrJSON)
	if !strings.HasPrefix(pathOrJSON, "{") {
		var err error
		if text, err = os.ReadFile(pathOrJSON); err != nil {
			return p, err
		}
	}
	return p, json.Unmarshal(text, &p)
}

// JSON is the policy as Load reads it given whole.
func (p Policy) JSON() (string, error) {
	text, err := json.Marshal(p)
	return string(text), err
}

// Wrap is the change to a command that runs it behind the policy in file, by the gusto program at self: `self sandbox-run FILE -- command`.
func Wrap(self, file string) func(*exec.Cmd) {
	return func(command *exec.Cmd) {
		command.Args = append([]string{self, "sandbox-run", file, "--"}, command.Args...)
		command.Path = self
	}
}
