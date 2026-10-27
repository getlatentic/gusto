package registry

import (
	"sort"
	"strings"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// View is the registry as a page says it: every provider, the operator's and the organization's, with the models
// of each. No key is in it, only where one is.
type View struct {
	Version   int64
	Providers []ProviderView
	// Kinds are the kinds of reference a key may be given as, "stored" first.
	Kinds []string
	// Private says whether a provider may be at a private address.
	Private bool
	// Limits are the people who may use only some of the models, and Available the models there are to choose from.
	Limits    []Access
	Available []string
	// Editable says whether the admin may change what is allowed, which is the two settings Private and Kinds say.
	Editable bool
	// Unnamed is set when the operator's own provider has no name, which an organization cannot add beside.
	Unnamed bool
}

// ProviderView is one provider.
type ProviderView struct {
	Name string
	// Addresses are where the provider answers, the first being the one it was added with.
	Addresses []AddressView
	// Operator says the deployment's own configuration made it, which is not changed here.
	Operator bool
	Retired  bool
	// Pending says the provider was added and is waiting for an administrator to turn it on.
	Pending bool
	// Key is where its key is: "stored", a reference, or "the operator's" for an operator's provider.
	Key string
	// Problem is why a provider of the organization's is not in use.
	Problem string
	Models  []ModelView
}

// AddressView is one of a provider's addresses and the kind of API it answers there.
type AddressView struct {
	Address string
	Kind    string
	// Extra says it was added beside the provider's first address, and can be taken away.
	Extra bool
}

// ModelView is one model of a provider.
type ModelView struct {
	Name    string
	Full    string
	Price   *spend.Price
	Retired bool
	Pending bool
}

// View is what a page shows now.
func (r *Registry) View() View {
	effective := r.Effective()
	document := r.Document()
	rules := r.Rules()
	view := View{
		Version: document.Version, Kinds: append([]string{"stored"}, rules.KeySources...),
		Private: rules.PrivateAddresses, Unnamed: r.platform.Credentials[""].Address != "", Editable: r.editable,
		Limits: r.Limits(), Available: effective.Models,
	}
	for _, name := range r.platform.Credentials.Names() {
		// A provider of the operator's that offers no model is not something an admin can use, so it is not shown.
		if row := r.operators(name, effective); len(row.Models) > 0 {
			view.Providers = append(view.Providers, row)
		}
	}
	for _, provider := range document.Providers {
		row := ProviderView{
			Name: provider.Name, Addresses: addressesOf(provider.Address, provider.Auth, provider.Wires, provider.Endpoints),
			Retired: provider.Retired, Pending: provider.Pending, Key: provider.Key, Problem: effective.Problems[provider.Name],
		}
		for _, model := range document.Models {
			if model.Provider == provider.Name {
				row.Models = append(row.Models, ModelView{Name: model.Name, Full: model.Full(), Price: model.Price, Retired: model.Retired, Pending: model.Pending})
			}
		}
		view.Providers = append(view.Providers, row)
	}
	return view
}

func (r *Registry) operators(name string, effective Effective) ProviderView {
	held := r.platform.Credentials[name]
	row := ProviderView{Name: name, Addresses: addressesOf(held.Address, held.Auth, held.Wires, held.Endpoints), Operator: true, Key: "the operator's"}
	for _, full := range r.platform.Models {
		if credential, model := proxy.Named(full); credential == name {
			entry := ModelView{Name: model, Full: full}
			if price, priced := effective.Prices[full]; priced {
				entry.Price = &price
			}
			row.Models = append(row.Models, entry)
		}
	}
	sort.SliceStable(row.Models, func(i, j int) bool { return row.Models[i].Name < row.Models[j].Name })
	return row
}

func addressesOf(address string, auth proxy.Auth, wires []string, others []proxy.Endpoint) []AddressView {
	all := []AddressView{{Address: address, Kind: KindOf(auth, wires)}}
	for _, at := range others {
		all = append(all, AddressView{Address: at.Address, Kind: KindOf(at.Auth, at.Wires), Extra: true})
	}
	return all
}

// KindOf is the kind of API an address answers, in the words the admin chose it by: OpenAI-compatible and
// Anthropic-compatible are the two that name no shapes, and anything else is custom.
func KindOf(auth proxy.Auth, wires []string) string {
	switch {
	case len(wires) == 0 && auth == proxy.Anthropic:
		return "Anthropic-compatible"
	case len(wires) == 0:
		return "OpenAI-compatible"
	}
	header := "bearer token"
	if auth == proxy.Anthropic {
		header = "Anthropic's header"
	}
	return "custom: " + strings.Join(wires, ", ") + ", " + header
}
