package registry

import (
	"maps"
	"slices"

	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/spend"
)

// Effective is what the registry comes to: what the proxy holds, what is offered and what a model is priced at.
type Effective struct {
	Version     int64
	Credentials proxy.Credentials
	// Models are full names, the operator's first.
	Models []string
	Prices map[string]spend.Price
	// Problems say why a provider of the organisation's is not in use, by its name.
	Problems map[string]string
}

// Effective is the registry and the operator's together. A provider whose key cannot be read is left out, with the
// reason, and so are its models, so a job is never given one that cannot be reached.
func (r *Registry) Effective() Effective {
	document := r.Document()
	held := maps.Clone(r.platform.Credentials)
	if held == nil {
		held = proxy.Credentials{}
	}
	out := Effective{
		Version: document.Version, Credentials: held,
		Models:   slices.Clone(r.platform.Models),
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
			Name: provider.Name, Address: provider.Address, Key: key, Auth: provider.Auth, Wires: provider.Wires, Public: !rules.PrivateAddresses,
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
