package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/pkg/proxy"
)

// wayOf is how the form says a provider is spoken to at an address: OpenAI-compatible, Anthropic-compatible, or custom,
// where the admin names the header the key is sent in and the shapes of API it answers.
func wayOf(r *http.Request) (proxy.Endpoint, error) {
	switch r.FormValue("kind") {
	case "", "openai":
		return proxy.Endpoint{Auth: proxy.Bearer}, nil
	case "anthropic":
		return proxy.Endpoint{Auth: proxy.Anthropic}, nil
	case "custom":
		if len(r.Form["wires"]) == 0 {
			return proxy.Endpoint{}, errors.New("a custom address answers at least one shape of API")
		}
		return proxy.Endpoint{Auth: proxy.Auth(r.FormValue("auth")), Wires: r.Form["wires"]}, nil
	}
	return proxy.Endpoint{}, errors.New("an address is OpenAI-compatible, Anthropic-compatible or custom")
}

func (s *Server) addAddress(r *http.Request) (map[string]string, error) {
	name := r.PathValue("name")
	at, err := wayOf(r)
	if err != nil {
		return nil, err
	}
	at.Address = r.FormValue("address")
	if err := s.registry.AddAddress(name, at); err != nil {
		return nil, err
	}
	return map[string]string{"provider": name, "address": strings.TrimSpace(at.Address), "kind": registry.KindOf(at.Auth, at.Wires)}, nil
}

func (s *Server) removeAddress(r *http.Request) (map[string]string, error) {
	name, address := r.PathValue("name"), r.FormValue("address")
	if err := s.registry.RemoveAddress(name, address); err != nil {
		return nil, err
	}
	return map[string]string{"provider": name, "address": address}, nil
}
