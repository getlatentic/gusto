package registry

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/spend"
)

var (
	providerName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	modelName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
)

// ErrNoSuch is a provider or a model that is not in the registry.
var ErrNoSuch = errors.New("there is no such provider or model")

// maxPrice is a price per million tokens no provider charges, so a slip of the hand is caught.
const maxPrice = 10000.0

// Give is what an admin says to add a provider. Key is what they typed when the key is to be kept here, and is
// kept and forgotten by this call.
type Give struct {
	Name, Address string
	Auth          proxy.Auth
	// Reference is "stored", or where the key is read from.
	Reference string
	Key       string
	Harnesses []string
}

// AddProvider adds a provider, or replaces one that was retired.
func (r *Registry) AddProvider(give Give) error {
	provider, err := r.provider(give)
	if err != nil {
		return err
	}
	if held := r.Document(); indexOfProvider(held.Providers, provider.Name) >= 0 && !held.Providers[indexOfProvider(held.Providers, provider.Name)].Retired {
		return fmt.Errorf("there is a provider called %s already", provider.Name)
	}
	if give.Reference == "stored" {
		if err := r.keep(provider.Name, give.Key); err != nil {
			return err
		}
	} else if give.Key != "" {
		return errors.New("a key is typed only for a provider whose key is kept here")
	}
	err = r.change(func(d *Document) error {
		if i := indexOfProvider(d.Providers, provider.Name); i >= 0 {
			if !d.Providers[i].Retired {
				return fmt.Errorf("there is a provider called %s already", provider.Name)
			}
			d.Providers[i] = provider
			// What the retired provider served is not what this one does.
			for j := range d.Models {
				if d.Models[j].Provider == provider.Name {
					d.Models[j].Retired = true
				}
			}
			return nil
		}
		d.Providers = append(d.Providers, provider)
		return nil
	})
	if err != nil && give.Reference == "stored" {
		r.forget(provider.Name)
	}
	return err
}

func (r *Registry) provider(give Give) (Provider, error) {
	name := strings.TrimSpace(give.Name)
	switch {
	case !providerName.MatchString(name):
		return Provider{}, errors.New("a provider's name is lower case letters, digits and hyphens, starting with a letter, up to 31")
	case r.platformHas(name):
		return Provider{}, fmt.Errorf("%s is the name of one of this deployment's own providers", name)
	case r.platform.Credentials[""].Address != "":
		return Provider{}, errors.New("this deployment names no provider for its own models, so one cannot be added beside it until it does")
	}
	address, err := r.checkAddress(give.Address)
	if err != nil {
		return Provider{}, err
	}
	if give.Auth != "" && give.Auth != proxy.Bearer && give.Auth != proxy.Anthropic {
		return Provider{}, errors.New("a provider takes a bearer token or Anthropic's header")
	}
	if err := Check(give.Reference, r.Rules().KeySources); err != nil {
		return Provider{}, err
	}
	for _, harness := range give.Harnesses {
		if !slices.Contains(r.Rules().Harnesses, harness) {
			return Provider{}, fmt.Errorf("%s is not a harness this deployment has", harness)
		}
	}
	harnesses := slices.Clone(give.Harnesses)
	slices.Sort(harnesses)
	return Provider{Name: name, Address: address, Auth: give.Auth, Key: give.Reference, Harnesses: slices.Compact(harnesses), Pending: true}, nil
}

func (r *Registry) platformHas(name string) bool {
	_, held := r.platform.Credentials[name]
	return held
}

func indexOfProvider(list []Provider, name string) int {
	return slices.IndexFunc(list, func(p Provider) bool { return p.Name == name })
}

// ReplaceKey gives a provider whose key is kept here another key.
func (r *Registry) ReplaceKey(provider, key string) error {
	held := r.Document()
	i := indexOfProvider(held.Providers, provider)
	if i < 0 || held.Providers[i].Retired {
		return ErrNoSuch
	}
	if held.Providers[i].Key != "stored" {
		return errors.New("that provider's key is not kept here")
	}
	if err := r.keep(provider, key); err != nil {
		return err
	}
	return r.change(func(*Document) error { return nil })
}

// RetireProvider stops a provider and its models being used, and forgets a key that was kept here.
func (r *Registry) RetireProvider(name string) error {
	err := r.change(func(d *Document) error {
		i := indexOfProvider(d.Providers, name)
		if i < 0 || d.Providers[i].Retired {
			return ErrNoSuch
		}
		d.Providers[i].Retired = true
		return nil
	})
	if err == nil {
		r.forget(name)
	}
	return err
}

// AddModel adds a model to a provider, or brings back one that was retired.
func (r *Registry) AddModel(provider, name string, price *spend.Price) error {
	name = strings.TrimSpace(name)
	if !modelName.MatchString(name) {
		return errors.New("a model is named as its provider names it: letters, digits and . _ : / -")
	}
	if err := checkPrice(price); err != nil {
		return err
	}
	return r.change(func(d *Document) error {
		if i := indexOfProvider(d.Providers, provider); i < 0 || d.Providers[i].Retired {
			return ErrNoSuch
		}
		if i := indexOfModel(d.Models, provider, name); i >= 0 {
			if !d.Models[i].Retired {
				return fmt.Errorf("%s/%s is in the registry already", provider, name)
			}
			d.Models[i] = Model{Provider: provider, Name: name, Price: price, Pending: true}
			return nil
		}
		d.Models = append(d.Models, Model{Provider: provider, Name: name, Price: price, Pending: true})
		return nil
	})
}

// Enable turns a provider on, or one of its models when a model is named. What an administrator adds is off until this: finding
// or typing something in does not offer it to people.
func (r *Registry) Enable(provider, model string) error { return r.setPending(provider, model, false) }

// Disable turns a provider or a model off again. It is kept, and can be turned on, which retiring it is not.
func (r *Registry) Disable(provider, model string) error { return r.setPending(provider, model, true) }

func (r *Registry) setPending(provider, model string, pending bool) error {
	return r.change(func(d *Document) error {
		if model == "" {
			i := indexOfProvider(d.Providers, provider)
			if i < 0 || d.Providers[i].Retired {
				return ErrNoSuch
			}
			d.Providers[i].Pending = pending
			return nil
		}
		i := indexOfModel(d.Models, provider, model)
		if i < 0 || d.Models[i].Retired {
			return ErrNoSuch
		}
		d.Models[i].Pending = pending
		return nil
	})
}

// SetPrice changes what a model costs, from the next call on.
func (r *Registry) SetPrice(provider, name string, price *spend.Price) error {
	if err := checkPrice(price); err != nil {
		return err
	}
	return r.change(func(d *Document) error {
		i := indexOfModel(d.Models, provider, name)
		if i < 0 || d.Models[i].Retired {
			return ErrNoSuch
		}
		d.Models[i].Price = price
		return nil
	})
}

// RetireModel stops a model being offered.
func (r *Registry) RetireModel(provider, name string) error {
	return r.change(func(d *Document) error {
		i := indexOfModel(d.Models, provider, name)
		if i < 0 || d.Models[i].Retired {
			return ErrNoSuch
		}
		d.Models[i].Retired = true
		return nil
	})
}

func indexOfModel(list []Model, provider, name string) int {
	return slices.IndexFunc(list, func(m Model) bool { return m.Provider == provider && m.Name == name })
}

func checkPrice(p *spend.Price) error {
	if p == nil {
		return nil
	}
	for _, rate := range []*float64{&p.InputPerMillion, &p.OutputPerMillion, p.CachedInputPerMillion, p.CacheWritePerMillion} {
		if rate != nil && (math.IsNaN(*rate) || *rate < 0 || *rate > maxPrice) {
			return errors.New("a price is dollars for a million tokens, from 0 to 10000")
		}
	}
	if p.Peak != nil {
		return errors.New("an organisation does not set peak prices")
	}
	return nil
}
