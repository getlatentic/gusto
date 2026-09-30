// Package registry is the models an organisation may use, and where each answers: one place that says what a
// model is called, where it lives, what it costs and which harnesses are known to work with it.
//
// The operator's own models, from the deployment's settings, are always there and cannot be changed from here.
// An organisation adds providers and models of its own, which are kept in one file in its data directory. The
// two together are what the proxy holds, what the forms offer and what the ledger prices at, and the registry
// has a version that goes up with every change, which each call the proxy makes is written with.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/getlatentic/haas/internal/atomicfile"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/spend"
)

// Provider is where models answer and how to be let in. Its key is never kept here: Key says where it is.
type Provider struct {
	Name      string     `json:"name"`
	Address   string     `json:"address"`
	Auth      proxy.Auth `json:"auth,omitempty"`
	Key       string     `json:"key"`
	Harnesses []string   `json:"harnesses,omitempty"`
	Retired   bool       `json:"retired,omitempty"`
}

// Model is one the provider serves, by the name the provider gives it.
type Model struct {
	Provider string       `json:"provider"`
	Name     string       `json:"name"`
	Price    *spend.Price `json:"price,omitempty"`
	Retired  bool         `json:"retired,omitempty"`
}

// Full is the name a job asks for: the provider, then the model.
func (m Model) Full() string { return m.Provider + "/" + m.Name }

// Policy is what an admin has chosen to allow, for a deployment where the admin is the operator.
type Policy struct {
	PrivateAddresses bool     `json:"privateAddresses,omitempty"`
	KeySources       []string `json:"keySources,omitempty"`
}

// Document is what is kept.
type Document struct {
	Version int64 `json:"version"`
	// Policy is set once an admin has chosen. It is used only where the registry is editable.
	Policy    *Policy    `json:"policy,omitempty"`
	Providers []Provider `json:"providers"`
	Models    []Model    `json:"models"`
	// Access is the people who may use only some of the models.
	Access []Access `json:"access,omitempty"`
}

// Platform is what the operator configured, which an organisation's registry is added to.
type Platform struct {
	Credentials proxy.Credentials
	// Models are full names, in the order they are offered.
	Models []string
	Prices map[string]spend.Price
}

// Rules are what an organisation's admin may and may not do. The operator sets them, and where the registry is
// editable, which is a deployment for one company whose admin is its operator, an admin changes the two that are
// about trust from the console.
type Rules struct {
	// Harnesses are the names a provider may be said to serve.
	Harnesses []string
	// PrivateAddresses lets a provider be at an address on a private network, or this machine, and
	// at http. A deployment for one company, which runs a model inside its own network, allows it.
	PrivateAddresses bool
	// KeySources are the kinds of reference a key may be given as, besides "stored", which is a
	// key the admin typed once and which is never shown again. "file", "ssm" and "secretsmanager" let an
	// admin name anything this machine can read, so a deployment that hosts other people's organisations
	// leaves them out.
	KeySources []string
}

// Registry is an organisation's registry, kept in a file, with the operator's added.
type Registry struct {
	path     string
	keys     string
	platform Platform
	rules    Rules
	editable bool
	run      func(name string, args ...string) ([]byte, error)
	resolve  Resolver

	mu       sync.Mutex
	document Document
	listener func(Effective)
}

// Open is the registry kept in dir, which is empty until something is added. A file that is there and cannot
// be read is an error and not an empty registry: a page that showed nothing would be wrong and look right.
func Open(dir string, platform Platform, rules Rules, editable bool) (*Registry, error) {
	r := &Registry{
		path: filepath.Join(dir, "registry.json"), keys: filepath.Join(dir, "keys"),
		platform: platform, rules: rules, editable: editable, resolve: systemResolver,
	}
	text, err := os.ReadFile(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the registry in %s cannot be read: %w", r.path, err)
	}
	if err := json.Unmarshal(text, &r.document); err != nil {
		return nil, fmt.Errorf("the registry in %s is not valid: %w", r.path, err)
	}
	return r, nil
}

// OnChange is told what the registry is whenever a change is kept, which is how the proxy, the forms and the
// ledger come to use it.
func (r *Registry) OnChange(listener func(Effective)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listener = listener
}

// Document is what is kept now, copied.
func (r *Registry) Document() Document {
	r.mu.Lock()
	defer r.mu.Unlock()
	return clone(r.document)
}

func clone(d Document) Document {
	d.Providers = slices.Clone(d.Providers)
	d.Models = slices.Clone(d.Models)
	d.Access = slices.Clone(d.Access)
	return d
}

// change makes one change, keeps the result, and tells the listener. A change that is refused leaves the registry
// as it was.
func (r *Registry) change(make func(*Document) error) error {
	r.mu.Lock()
	next := clone(r.document)
	if err := make(&next); err != nil {
		r.mu.Unlock()
		return err
	}
	next.Version++
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		r.mu.Unlock()
		return err
	}
	if err := atomicfile.Write(r.path, next, 0o600); err != nil {
		r.mu.Unlock()
		return err
	}
	r.document = next
	listener := r.listener
	r.mu.Unlock()
	if listener != nil {
		listener(r.Effective())
	}
	return nil
}

// Rules are what an admin may do now: the operator's, with the two that are about trust replaced by the admin's
// choice where the registry is editable and one has been made.
func (r *Registry) Rules() Rules {
	rules := r.rules
	if !r.editable {
		return rules
	}
	if chosen := r.Document().Policy; chosen != nil {
		rules.PrivateAddresses, rules.KeySources = chosen.PrivateAddresses, slices.Clone(chosen.KeySources)
	}
	return rules
}

// Editable says whether the admin may change the policy.
func (r *Registry) Editable() bool { return r.editable }

// ErrFixed is a policy that the operator fixed.
var ErrFixed = errors.New("this deployment's operator fixes that, so it is not changed here")

// SetPolicy changes what an admin allows: providers at private addresses and plain http, and the kinds of
// reference a key may be named as. Only a deployment that is not hosted allows it, since elsewhere the admin
// is one of many and these are what keep the machine safe from them.
func (r *Registry) SetPolicy(policy Policy) error {
	if !r.editable {
		return ErrFixed
	}
	for _, kind := range policy.KeySources {
		if !slices.Contains([]string{"file", "ssm", "secretsmanager"}, kind) {
			return fmt.Errorf("%q is not a place a key is read from", kind)
		}
	}
	slices.Sort(policy.KeySources)
	policy.KeySources = slices.Compact(policy.KeySources)
	return r.change(func(d *Document) error {
		d.Policy = &policy
		return nil
	})
}
