package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Auth is how a credential is presented to its provider. Most take a bearer
// token; Anthropic takes its own header and a version alongside it.
type Auth string

const (
	Bearer    Auth = "bearer"
	Anthropic Auth = "anthropic"
)

// AnthropicVersion is the version header Anthropic requires. It is sent only
// when the container did not send one, so a harness that knows better wins.
const AnthropicVersion = "2023-06-01"

// Credential is where a model answers and what it takes to be let in.
//
// A deployment holds several, because one provider's key does not reach
// another's models and two providers do not authenticate the same way. Which
// one a job may spend is decided when the job starts and carried by its token,
// never by what the container puts in a request body.
type Credential struct {
	// Name is what the evidence records. The key itself never appears anywhere.
	Name string `json:"name"`
	// Address is the model's root, with no version on it: the container is given
	// `.../model/<token>/v1` and the harness appends the rest of the path, so
	// this must not repeat it.
	Address string `json:"address"`
	Key     string `json:"key,omitempty"`
	Auth    Auth   `json:"auth,omitempty"`
	// Harnesses is which harnesses this credential can actually serve, where
	// that is narrower than all of them. Naming none means all of them, which
	// is the ordinary case; naming some is how a deployment records that a
	// provider refuses what a particular harness sends it.
	Harnesses []string `json:"harnesses,omitempty"`
}

// Serves reports whether this credential will answer that harness. A credential
// naming no harnesses answers every one, so a deployment says nothing until it
// has found out otherwise.
func (c Credential) Serves(harness string) bool {
	if len(c.Harnesses) == 0 {
		return true
	}
	for _, named := range c.Harnesses {
		if named == harness {
			return true
		}
	}
	return false
}

// apply puts this credential on a request and takes off whatever the container
// sent in its place. Both headers are cleared either way, so a container cannot
// reach a provider by presenting a header the chosen credential does not use.
func (c Credential) apply(header http.Header) {
	header.Del("Authorization")
	header.Del("x-api-key")
	if c.Key == "" {
		return
	}
	if c.Auth == Anthropic {
		header.Set("x-api-key", c.Key)
		if header.Get("anthropic-version") == "" {
			header.Set("anthropic-version", AnthropicVersion)
		}
		return
	}
	header.Set("Authorization", "Bearer "+c.Key)
}

// Credentials is every credential a deployment holds, by name.
type Credentials map[string]Credential

// Get is the credential of that name.
//
// A deployment configured the older way holds one credential with no name, and
// that one serves every job whatever its model says — which is what the single
// upstream did. A deployment that names its credentials is strict: a name it
// does not hold is not quietly served by another.
func (c Credentials) Get(name string) (Credential, bool) {
	if held, found := c[name]; found {
		return held, true
	}
	if only, found := c[""]; found && len(c) == 1 {
		return only, true
	}
	return Credential{}, false
}

// Names is what this deployment holds, in order, for an error that has to say.
func (c Credentials) Names() []string {
	names := make([]string, 0, len(c))
	for name := range c {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Named splits `<credential>/<model>` into the credential that serves a model
// and the model itself. A model with no credential on it names none, and is
// served by the only credential there is.
func Named(model string) (credential, rest string) {
	if before, after, found := strings.Cut(model, "/"); found {
		return before, after
	}
	return "", model
}

// OpenCredentials reads every <name>.json in a directory, the way the keyring
// and the adapters are read. The file name is the credential's name, so two
// files cannot disagree about what one is called.
func OpenCredentials(dir string) (Credentials, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	held := Credentials{}
	for _, path := range paths {
		text, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var credential Credential
		if err := json.Unmarshal(text, &credential); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		credential.Name = strings.TrimSuffix(filepath.Base(path), ".json")
		if credential.Address == "" {
			return nil, fmt.Errorf("%s: a credential needs the address its model answers at", credential.Name)
		}
		held[credential.Name] = credential
	}
	return held, nil
}
