package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/getlatentic/gusto/pkg/wire"
)

// Auth is how a credential is presented to its provider. Most take a bearer
// token; Anthropic takes its own header and a version alongside it.
type Auth string

const (
	Bearer    Auth = "bearer"
	Anthropic Auth = "anthropic"
)

// AnthropicVersion is the version header Anthropic requires. It is sent only
// when the container did not send one, so an agent that knows better wins.
const AnthropicVersion = "2023-06-01"

// Credential is where a model answers and what it takes to be let in.
//
// A deployment holds several, because one provider's key does not reach
// another's models and two providers do not authenticate the same way. Which
// one a task may spend is decided when the task starts and carried by its token,
// never by what the container puts in a request body.
type Credential struct {
	// Name is what the record records. The key itself never appears anywhere.
	Name string `json:"name"`
	// Address is the model's root, with no version on it: the container is given
	// `.../model/<token>/v1` and the agent appends the rest of the path, so
	// this must not repeat it.
	Address string `json:"address"`
	Key     string `json:"key,omitempty"`
	Auth    Auth   `json:"auth,omitempty"`
	// Wires is the shapes of API the provider answers in: "chat", "messages" or "responses". A call in a shape it
	// does not answer is translated into Chat Completions when it answers that, so that any agent can use its models.
	// Naming none is a provider that answers what its authentication says, and nothing is translated for it.
	Wires []string `json:"wires,omitempty"`
	// MaxOutput is the most the provider lets a model write in one answer. A translated call that asks for more is
	// held to it, since an agent asks for what its own provider allows. Zero holds no call to anything.
	MaxOutput int `json:"maxOutput,omitempty"`
	// Endpoints are the provider's other addresses, under the same key, each answering its own shapes: DeepSeek answers
	// OpenAI's shapes at one address and Anthropic's at another. A call goes to the first address that answers its shape.
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	// Public marks a credential an organization added for itself. Its address is reached only at a public
	// address, since the organization chose it and this machine is on a network that others are on too.
	Public bool `json:"-"`
}

// Endpoint is one more address of a credential's provider, and the way it is spoken to there.
type Endpoint struct {
	Address string   `json:"address"`
	Auth    Auth     `json:"auth,omitempty"`
	Wires   []string `json:"wires,omitempty"`
}

// Answers says whether the provider takes a call in that shape as it is, at any of its addresses.
func (c Credential) Answers(shape wire.Shape) bool {
	for _, at := range c.addresses() {
		if at.answersHere(shape) {
			return true
		}
	}
	return false
}

// answersHere is Answers at the credential's own address. One that names its wires answers those; one that does not
// answers Anthropic's Messages when it is authenticated the Anthropic way, and OpenAI's two shapes otherwise.
func (c Credential) answersHere(shape wire.Shape) bool {
	if len(c.Wires) > 0 {
		return slices.Contains(c.Wires, string(shape))
	}
	if c.Auth == Anthropic {
		return shape == wire.Messages
	}
	return shape == wire.Chat || shape == wire.Responses
}

// For is the credential at the address a call in that shape goes to: the first that answers it, else the first that
// answers Chat Completions, which the call is translated into, else the credential's own.
func (c Credential) For(shape wire.Shape) Credential {
	addresses := c.addresses()
	for _, want := range []wire.Shape{shape, wire.Chat} {
		for _, at := range addresses {
			if at.answersHere(want) {
				return at
			}
		}
	}
	return addresses[0]
}

// Addresses is every address a call through this credential may go to, its own first.
func (c Credential) Addresses() []string {
	all := []string{c.Address}
	for _, at := range c.Endpoints {
		all = append(all, at.Address)
	}
	return all
}

// addresses is the credential at each of its addresses, its own first, each with no further endpoints.
func (c Credential) addresses() []Credential {
	own := c
	own.Endpoints = nil
	all := []Credential{own}
	for _, at := range c.Endpoints {
		other := own
		other.Address, other.Auth, other.Wires = at.Address, at.Auth, at.Wires
		all = append(all, other)
	}
	return all
}

// Translates says whether a call in that shape is translated into Chat Completions for this provider: one that answers
// Chat Completions and not this shape, so that every engine can use every model a key reaches.
func (c Credential) Translates(shape wire.Shape) bool {
	return (shape == wire.Messages || shape == wire.Responses) && !c.Answers(shape) && c.Answers(wire.Chat)
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
// that one serves every task whatever its model says — which is what the single
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
		for _, at := range credential.Endpoints {
			if at.Address == "" {
				return nil, fmt.Errorf("%s: each of a credential's endpoints needs its address", credential.Name)
			}
		}
		held[credential.Name] = credential
	}
	return held, nil
}
