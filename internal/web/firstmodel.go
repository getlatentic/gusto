package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/registry"
)

// A person on their own computer starts with no model, and adds the one they have a key for: where it answers, whether it
// answers the way OpenAI's API does or Anthropic's, the key, and the models to use. Every engine is offered every model.

// modelPreset is a provider a person likely has a key for, so the form can fill in where it answers and how.
type modelPreset struct {
	Title, Address string
	Auth           proxy.Auth
}

var modelPresets = []modelPreset{
	{"Anthropic", "https://api.anthropic.com", proxy.Anthropic},
	{"OpenAI", "https://api.openai.com", proxy.Bearer},
	{"DeepSeek", "https://api.deepseek.com", proxy.Bearer},
	{"OpenRouter", "https://openrouter.ai/api", proxy.Bearer},
	{"Ollama on this computer", "http://127.0.0.1:11434", proxy.Bearer},
}

// UseFoundKeys names the model keys the person's shell had when haas was installed, which the chat page offers.
func (s *Server) UseFoundKeys(names []string) { s.foundKeys = names }

// needsModel says whether this person's own haas has no model yet, so the chat page asks for one in place of a box nothing
// can answer.
func (s *Server) needsModel(r *http.Request) bool {
	return s.ownsItsModels() && s.admin(r) && len(s.offered()) == 0
}

// missingEngines is the engines this computer does not have yet, each with how to install it, on a person's own haas.
func (s *Server) missingEngines() []catalog.Entry {
	if s.local == nil {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(s.engines()), func(entry catalog.Entry) bool { return entry.Ready })
}

// addFirstModel adds a provider and its models in one step, and turns them on: on a person's own computer the person who adds a
// model is the one who uses it, so there is nobody else to turn it on.
func (s *Server) addFirstModel(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, formLimit)
	if err := r.ParseForm(); err != nil {
		s.turnAway(w, r, "/chat", "that form is too large")
		return
	}
	what, err := s.addProviderWith(r.FormValue("address"), proxy.Auth(r.FormValue("auth")), r.FormValue("key"), modelNames(r.FormValue("models")))
	if err != nil {
		s.turnAway(w, r, "/chat", registryWords(err))
		return
	}
	s.note(r, changelog.ProviderAdded, what)
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}

func (s *Server) addProviderWith(address string, auth proxy.Auth, key string, models []string) (map[string]string, error) {
	if len(models) == 0 {
		return nil, errors.New("name at least one model to use, or press List my models")
	}
	name, err := s.providerNameFor(address)
	if err != nil {
		return nil, err
	}
	if err := s.registry.AddProvider(registry.Give{Name: name, Address: address, Auth: auth, Reference: "stored", Key: key, Wires: wiresOf(address, auth)}); err != nil {
		return nil, err
	}
	if err := s.registry.Enable(name, ""); err != nil {
		return nil, err
	}
	for _, model := range models {
		if err := s.registry.AddModel(name, model, nil); err != nil {
			return nil, err
		}
		if err := s.registry.Enable(name, model); err != nil {
			return nil, err
		}
	}
	return map[string]string{"provider": name, "address": strings.TrimSpace(address), "key": "stored", "models": strings.Join(models, ", ")}, nil
}

// wiresOf is the shapes of API a provider a person adds answers in. An OpenAI-compatible provider answers Chat Completions,
// and only OpenAI's own answers the Responses API too, so Claude Code's and Codex's calls are translated for the rest. An
// Anthropic-compatible one answers Messages.
func wiresOf(address string, auth proxy.Auth) []string {
	if auth == proxy.Anthropic {
		return nil
	}
	if parsed, err := url.Parse(strings.TrimSpace(address)); err == nil && parsed.Hostname() == "api.openai.com" {
		return []string{"chat", "responses"}
	}
	return []string{"chat"}
}

// providerNameFor names a provider after where it answers: api.deepseek.com is deepseek, a second one there deepseek-2, and
// one on this computer local.
func (s *Server) providerNameFor(address string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("that is not an address: it looks like https://api.example.com")
	}
	host := parsed.Hostname()
	base := host
	for _, label := range strings.Split(host, ".") {
		if label != "api" && label != "www" {
			base = label
			break
		}
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		base = "local"
	}
	base = strings.Trim(nameChars.ReplaceAllString(strings.ToLower(base), "-"), "-")
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "provider-" + base
	}
	base = strings.TrimRight(base[:min(len(base), 27)], "-")
	taken := func(name string) bool {
		return slices.ContainsFunc(s.registry.Document().Providers, func(p registry.Provider) bool { return p.Name == name && !p.Retired })
	}
	name := base
	for n := 2; taken(name); n++ {
		name = fmt.Sprintf("%s-%d", base, n)
	}
	return name, nil
}

var nameChars = regexp.MustCompile(`[^a-z0-9-]+`)

// modelNames reads the models a person typed, one or several, separated by commas, spaces or lines.
func modelNames(text string) []string {
	var names []string
	for _, name := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// listModels asks a provider which models a key can use, for the form to offer. It is for a person's own haas alone, since
// it calls the address the person gives it.
func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, formLimit)
	if err := r.ParseForm(); err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "that form is too large"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	models, err := providerModels(ctx, r.FormValue("address"), proxy.Auth(r.FormValue("auth")), r.FormValue("key"))
	if err != nil {
		answerJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	answerJSON(w, http.StatusOK, map[string][]string{"models": models})
}

// providerModels is what a provider answers to GET /v1/models, which OpenAI's API, Anthropic's and the ones like them all have.
func providerModels(ctx context.Context, address string, auth proxy.Auth, key string) ([]string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(address), "/") + "/v1/models")
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, errors.New("that is not an address: it looks like https://api.example.com")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	if auth == proxy.Anthropic {
		request.Header.Set("x-api-key", key)
		request.Header.Set("anthropic-version", proxy.AnthropicVersion)
	} else if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s did not answer: %w", parsed.Host, err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s: check the key, and whether it answers the way OpenAI's API does or Anthropic's", parsed.Host, answer.Status)
	}
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(answer.Body, 4<<20)).Decode(&listed); err != nil {
		return nil, fmt.Errorf("%s did not list its models: %w", parsed.Host, err)
	}
	var models []string
	for _, model := range listed.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	slices.Sort(models)
	return models, nil
}
