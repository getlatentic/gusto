package cloud

import (
	"path/filepath"
	"time"
)

// State is where an organisation's server is.
type State string

const (
	Provisioning State = "provisioning"
	Starting     State = "starting"
	Ready        State = "ready"
	// Waiting is an organisation no cell has room for yet, which is tried again in a few seconds.
	Waiting State = "waiting"
	Failed  State = "failed"
)

// Runtime is what the front door knows of an organisation's server: which cell it
// is on, the two ports it was given, and how it is doing. It is written by the
// controller alone, beside the organisation's record and apart from it.
type Runtime struct {
	State State  `json:"state"`
	Cell  string `json:"cell,omitempty"`
	// Port is where the organisation's server listens, and ProxyPort where its
	// proxy for models does, both on the cell's own loopback.
	Port      int       `json:"port,omitempty"`
	ProxyPort int       `json:"proxyPort,omitempty"`
	Error     string    `json:"error,omitempty"`
	Restarts  int       `json:"restarts,omitempty"`
	Updated   time.Time `json:"updated"`
}

func (s *Store) runtimeFile(slug string) string { return filepath.Join(s.OrgDir(slug), "runtime.json") }

// Runtime is the state of an organisation's server. One never provisioned has none.
func (s *Store) Runtime(slug string) (Runtime, error) {
	var runtime Runtime
	err := readFile(s.runtimeFile(slug), &runtime)
	if missing(err) {
		return Runtime{}, nil
	}
	return runtime, err
}

// SetRuntime keeps the state of an organisation's server.
func (s *Store) SetRuntime(slug string, runtime Runtime) error {
	runtime.Updated = s.now().UTC().Truncate(time.Second)
	return writeFile(s.runtimeFile(slug), runtime, 0o600)
}

// Orgs is every organisation there is.
func (s *Store) Orgs() ([]Org, error) {
	entries, err := readDir(filepath.Join(s.root, "orgs"))
	if err != nil {
		return nil, err
	}
	var orgs []Org
	for _, name := range entries {
		var org Org
		if err := readFile(filepath.Join(s.root, "orgs", name, "org.json"), &org); err == nil {
			orgs = append(orgs, org)
		}
	}
	return orgs, nil
}
