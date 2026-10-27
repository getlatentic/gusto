package registry

import (
	"errors"
	"fmt"
	"slices"

	"github.com/getlatentic/gusto/pkg/proxy"
)

// AddAddress gives a provider one more address under its key, for a kind of API it answers somewhere else: DeepSeek
// answers OpenAI's shapes at one address and Anthropic's at another, and is one provider with one key.
func (r *Registry) AddAddress(provider string, at proxy.Endpoint) error {
	address, err := r.checkAddress(at.Address)
	if err != nil {
		return err
	}
	if err := checkWay(at.Auth, at.Wires); err != nil {
		return err
	}
	at.Address = address
	return r.change(func(d *Document) error {
		i := indexOfProvider(d.Providers, provider)
		if i < 0 || d.Providers[i].Retired {
			return ErrNoSuch
		}
		held := &d.Providers[i]
		if held.Address == address || slices.ContainsFunc(held.Endpoints, func(e proxy.Endpoint) bool { return e.Address == address }) {
			return fmt.Errorf("%s is one of %s's addresses already", address, provider)
		}
		held.Endpoints = append(slices.Clone(held.Endpoints), at)
		return nil
	})
}

// RemoveAddress takes one of a provider's other addresses away. Its first address is the provider, and goes only with it.
func (r *Registry) RemoveAddress(provider, address string) error {
	return r.change(func(d *Document) error {
		i := indexOfProvider(d.Providers, provider)
		if i < 0 || d.Providers[i].Retired {
			return ErrNoSuch
		}
		held := &d.Providers[i]
		j := slices.IndexFunc(held.Endpoints, func(e proxy.Endpoint) bool { return e.Address == address })
		if j < 0 {
			return ErrNoSuch
		}
		held.Endpoints = slices.Delete(slices.Clone(held.Endpoints), j, j+1)
		return nil
	})
}

// checkWay says whether this is a way a provider is spoken to: a bearer token or Anthropic's header, and the shapes of API it answers.
func checkWay(auth proxy.Auth, wires []string) error {
	if auth != "" && auth != proxy.Bearer && auth != proxy.Anthropic {
		return errors.New("a provider takes a bearer token or Anthropic's header")
	}
	for _, wire := range wires {
		if wire != "chat" && wire != "messages" && wire != "responses" {
			return fmt.Errorf("%s is not a shape of API: chat, messages or responses", wire)
		}
	}
	return nil
}
