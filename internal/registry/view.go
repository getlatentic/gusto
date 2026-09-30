package registry

import (
	"slices"
	"sort"

	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/spend"
)

// View is the registry as a page says it: every provider, the operator's and the organisation's, with the models
// of each. No key is in it, only where one is.
type View struct {
	Version   int64
	Providers []ProviderView
	// Kinds are the kinds of reference a key may be given as, "stored" first.
	Kinds []string
	// Harnesses are the names a provider may be said to serve.
	Harnesses []string
	// Private says whether a provider may be at a private address.
	Private bool
	// Unnamed is set when the operator's own provider has no name, which an organisation cannot add beside.
	Unnamed bool
}

// ProviderView is one provider.
type ProviderView struct {
	Name      string
	Address   string
	Auth      string
	Harnesses []string
	// Operator says the deployment's own configuration made it, which is not changed here.
	Operator bool
	Retired  bool
	// Key is where its key is: "stored", a reference, or "the operator's" for an operator's provider.
	Key string
	// Problem is why a provider of the organisation's is not in use.
	Problem string
	Models  []ModelView
}

// ModelView is one model of a provider.
type ModelView struct {
	Name    string
	Full    string
	Price   *spend.Price
	Retired bool
}

// View is what a page shows now.
func (r *Registry) View() View {
	effective := r.Effective()
	document := r.Document()
	view := View{
		Version: document.Version, Kinds: append([]string{"stored"}, r.rules.KeySources...), Harnesses: slices.Clone(r.rules.Harnesses),
		Private: r.rules.PrivateAddresses, Unnamed: r.platform.Credentials[""].Address != "",
	}
	for _, name := range r.platform.Credentials.Names() {
		view.Providers = append(view.Providers, r.operators(name, effective))
	}
	for _, provider := range document.Providers {
		row := ProviderView{
			Name: provider.Name, Address: provider.Address, Auth: authWords(provider.Auth), Harnesses: provider.Harnesses,
			Retired: provider.Retired, Key: provider.Key, Problem: effective.Problems[provider.Name],
		}
		for _, model := range document.Models {
			if model.Provider == provider.Name {
				row.Models = append(row.Models, ModelView{Name: model.Name, Full: model.Full(), Price: model.Price, Retired: model.Retired})
			}
		}
		view.Providers = append(view.Providers, row)
	}
	return view
}

func (r *Registry) operators(name string, effective Effective) ProviderView {
	held := r.platform.Credentials[name]
	row := ProviderView{Name: name, Address: held.Address, Auth: authWords(held.Auth), Harnesses: held.Harnesses, Operator: true, Key: "the operator's"}
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

func authWords(auth proxy.Auth) string {
	if auth == proxy.Anthropic {
		return "Anthropic's header"
	}
	return "bearer token"
}
