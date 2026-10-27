package registry

import (
	"maps"
	"slices"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// Effective is what the registry comes to: what the proxy holds, what is offered and what a model is priced at.
type Effective struct {
	Version     int64
	Credentials proxy.Credentials
	// Models are full names, the operator's first.
	Models []string
	Prices map[string]spend.Price
	// Problems say why a provider of the organization's is not in use, by its name.
	Problems map[string]string
}

// Effective is the registry and the operator's together. A provider whose key cannot be read is left out, with the
// reason, and so are its models, so a task is never given one that cannot be reached.
func (r *Registry) Effective() Effective {
	document := r.Document()
	held := maps.Clone(r.platform.Credentials)
	if held == nil {
		held = proxy.Credentials{}
	}
	out := Effective{
		Version: document.Version, Credentials: held,
		Models:   reachable(r.platform.Models, held),
		Prices:   maps.Clone(r.platform.Prices),
		Problems: map[string]string{},
	}
	if out.Prices == nil {
		out.Prices = map[string]spend.Price{}
	}
	rules := r.Rules()
	for _, provider := range document.Providers {
		if provider.Retired || provider.Pending {
			continue
		}
		key, err := r.read(provider)
		if err != nil {
			out.Problems[provider.Name] = err.Error()
			continue
		}
		out.Credentials[provider.Name] = proxy.Credential{
			Name: provider.Name, Address: provider.Address, Key: key, Auth: provider.Auth, Wires: provider.Wires,
			Endpoints: provider.Endpoints, Public: !rules.PrivateAddresses,
		}
	}
	for _, model := range document.Models {
		if _, usable := out.Credentials[model.Provider]; model.Retired || model.Pending || !usable {
			continue
		}
		out.Models = append(out.Models, model.Full())
		if model.Price != nil {
			out.Prices[model.Full()] = *model.Price
		}
	}
	return out
}

// reachable is the operator's models whose credential is held, so a model named in the settings whose credential was taken
// away is not offered. A deployment that holds no credentials reaches its models without the proxy, and keeps them all.
func reachable(models []string, held proxy.Credentials) []string {
	if len(held) == 0 {
		return slices.Clone(models)
	}
	var out []string
	for _, full := range models {
		credential, _ := proxy.Named(full)
		if _, found := held.Get(credential); found {
			out = append(out, full)
		}
	}
	return out
}
