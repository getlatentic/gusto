package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/registry"
	"github.com/getlatentic/haas/internal/roster"
	"github.com/getlatentic/haas/internal/spend"
)

// UseRegistry gives the console a page of the models the organisation may use, which its admins change, and
// says how the models this server offers are read once they have. Without it the console has no such page.
func (s *Server) UseRegistry(r *registry.Registry, models func() Models) {
	s.registry, s.live = r, models
}

func (s *Server) hasRegistry() bool { return s.registry != nil }

// formLimit is the most a form to the registry may be.
const formLimit = 16 << 10

func (s *Server) modelsPage(w http.ResponseWriter, r *http.Request) {
	view := s.registry.View()
	page := map[string]any{"Title": "haas — models", "View": view, "Refused": r.URL.Query().Get("refused"), "Sources": keySources(view), "People": s.memberEmails()}
	s.drawConsole(w, "admin-models.html", "models", page)
}

// registryAction reads a form, makes the change and records it, and goes back to the page with the reason when
// it is refused. What is recorded is what changed and never a key.
func (s *Server) registryAction(kind string, make func(r *http.Request) (map[string]string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, formLimit)
		if err := r.ParseForm(); err != nil {
			s.turnAway(w, r, "/admin/models", "that form is too large")
			return
		}
		what, err := make(r)
		if err != nil {
			s.turnAway(w, r, "/admin/models", registryWords(err))
			return
		}
		s.note(r, kind, what)
		http.Redirect(w, r, "/admin/models", http.StatusSeeOther)
	}
}

func registryWords(err error) string {
	if errors.Is(err, registry.ErrNoSuch) {
		return "that provider or model is not in the registry any more"
	}
	return err.Error()
}

func (s *Server) modelRoutes(mux *http.ServeMux) {
	guard := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasRegistry, next)) }
	mux.HandleFunc("GET /admin/models", guard(s.modelsPage))
	mux.HandleFunc("POST /admin/models/providers", guard(s.registryAction(changelog.ProviderAdded, s.addProvider)))
	mux.HandleFunc("POST /admin/models/providers/{name}/key", guard(s.registryAction(changelog.KeyReplaced, s.replaceKey)))
	mux.HandleFunc("POST /admin/models/providers/{name}/retire", guard(s.registryAction(changelog.ProviderRetired, s.retireProvider)))
	mux.HandleFunc("POST /admin/models/providers/{name}/models", guard(s.registryAction(changelog.ModelAdded, s.addModel)))
	mux.HandleFunc("POST /admin/models/providers/{name}/price", guard(s.registryAction(changelog.PriceSet, s.setPrice)))
	mux.HandleFunc("POST /admin/models/access", guard(s.registryAction(changelog.AccessSet, s.setAccess)))
	mux.HandleFunc("POST /admin/models/access/clear", guard(s.registryAction(changelog.AccessSet, s.clearAccess)))
	mux.HandleFunc("POST /admin/models/policy", guard(s.registryAction(changelog.PolicySet, s.setPolicy)))
	mux.HandleFunc("POST /admin/models/providers/{name}/models/retire", guard(s.registryAction(changelog.ModelRetired, s.retireModel)))
}

func (s *Server) addProvider(r *http.Request) (map[string]string, error) {
	reference := "stored"
	if kind := r.FormValue("source"); kind != "" && kind != "stored" {
		reference = kind + ":" + strings.TrimSpace(r.FormValue("reference"))
	}
	give := registry.Give{
		Name: r.FormValue("name"), Address: r.FormValue("address"), Auth: proxy.Auth(r.FormValue("auth")),
		Reference: reference, Key: r.FormValue("key"), Harnesses: r.Form["harness"],
	}
	if err := s.registry.AddProvider(give); err != nil {
		return nil, err
	}
	what := map[string]string{"provider": strings.TrimSpace(give.Name), "address": strings.TrimSpace(give.Address), "key": registry.Kind(reference)}
	if reference == "stored" {
		what["key"] = "stored"
	}
	return what, nil
}

func (s *Server) replaceKey(r *http.Request) (map[string]string, error) {
	name := r.PathValue("name")
	if err := s.registry.ReplaceKey(name, r.FormValue("key")); err != nil {
		return nil, err
	}
	return map[string]string{"provider": name}, nil
}

func (s *Server) retireProvider(r *http.Request) (map[string]string, error) {
	name := r.PathValue("name")
	if err := s.registry.RetireProvider(name); err != nil {
		return nil, err
	}
	return map[string]string{"provider": name}, nil
}

func (s *Server) addModel(r *http.Request) (map[string]string, error) {
	price, err := priceOf(r)
	if err != nil {
		return nil, err
	}
	provider, name := r.PathValue("name"), strings.TrimSpace(r.FormValue("model"))
	if err := s.registry.AddModel(provider, name, price); err != nil {
		return nil, err
	}
	return map[string]string{"model": provider + "/" + name, "price": priceWords(price)}, nil
}

func (s *Server) setPrice(r *http.Request) (map[string]string, error) {
	price, err := priceOf(r)
	if err != nil {
		return nil, err
	}
	provider, name := r.PathValue("name"), r.FormValue("model")
	if err := s.registry.SetPrice(provider, name, price); err != nil {
		return nil, err
	}
	return map[string]string{"model": provider + "/" + name, "price": priceWords(price)}, nil
}

func (s *Server) retireModel(r *http.Request) (map[string]string, error) {
	provider, name := r.PathValue("name"), r.FormValue("model")
	if err := s.registry.RetireModel(provider, name); err != nil {
		return nil, err
	}
	return map[string]string{"model": provider + "/" + name}, nil
}

// priceOf is the price a form says, in dollars for a million tokens. Nothing typed for the input and the output is
// no price, which is an unknown cost and not a free model.
func priceOf(r *http.Request) (*spend.Price, error) {
	in, out, cached := strings.TrimSpace(r.FormValue("input")), strings.TrimSpace(r.FormValue("output")), strings.TrimSpace(r.FormValue("cached"))
	if in == "" && out == "" && cached == "" {
		return nil, nil
	}
	if in == "" || out == "" {
		return nil, errors.New("a price needs the input and the output, in dollars for a million tokens")
	}
	price := spend.Price{}
	var err error
	if price.InputPerMillion, err = rate(in); err == nil {
		price.OutputPerMillion, err = rate(out)
	}
	if err == nil && cached != "" {
		var held float64
		if held, err = rate(cached); err == nil {
			price.CachedInputPerMillion = &held
		}
	}
	if err != nil {
		return nil, err
	}
	return &price, nil
}

func rate(typed string) (float64, error) {
	value, err := strconv.ParseFloat(typed, 64)
	if err != nil {
		return 0, errors.New("a price is a number of dollars for a million tokens")
	}
	return value, nil
}

func priceWords(p *spend.Price) string {
	if p == nil {
		return "none"
	}
	return fmt.Sprintf("$%g in, $%g out", p.InputPerMillion, p.OutputPerMillion)
}

// setPolicy is what an admin allows of providers, where the deployment leaves that to them.
func (s *Server) setPolicy(r *http.Request) (map[string]string, error) {
	policy := registry.Policy{PrivateAddresses: r.FormValue("private") == "on", KeySources: r.Form["keys"]}
	if err := s.registry.SetPolicy(policy); err != nil {
		return nil, err
	}
	what := map[string]string{"private": "no", "keys": strings.Join(s.registry.Rules().KeySources, ", ")}
	if policy.PrivateAddresses {
		what["private"] = "yes"
	}
	return what, nil
}

// keySource is one kind of reference a key may be named as, for the policy form.
type keySource struct {
	Kind, Words string
	On          bool
}

func keySources(view registry.View) []keySource {
	all := []keySource{{Kind: "file", Words: "a file on this machine"}, {Kind: "ssm", Words: "an AWS Parameter Store parameter"}, {Kind: "secretsmanager", Words: "an AWS Secrets Manager secret"}}
	for i := range all {
		all[i].On = slices.Contains(view.Kinds, all[i].Kind)
	}
	return all
}

// setAccess limits a person to the models an admin ticked. Ticking none is no limit, which is said on the form.
func (s *Server) setAccess(r *http.Request) (map[string]string, error) {
	email, models := strings.TrimSpace(r.FormValue("email")), r.Form["model"]
	if len(models) == 0 {
		return nil, errors.New("tick the models this person may use, or use Let use everything to remove a limit")
	}
	if err := s.registry.SetAccess(email, models); err != nil {
		return nil, err
	}
	slices.Sort(models)
	return map[string]string{"email": strings.ToLower(email), "models": strings.Join(slices.Compact(models), ", ")}, nil
}

func (s *Server) clearAccess(r *http.Request) (map[string]string, error) {
	email := strings.TrimSpace(r.FormValue("email"))
	if err := s.registry.SetAccess(email, nil); err != nil {
		return nil, err
	}
	return map[string]string{"email": strings.ToLower(email)}, nil
}

// memberEmails is who is in the organisation, for the form that limits a person, or nobody when the list cannot be read.
func (s *Server) memberEmails() []string {
	if !s.hasRoster() {
		return nil
	}
	list, err := roster.Read(s.rosterPath)
	if err != nil {
		return nil
	}
	emails := make([]string, 0, len(list.Members))
	for _, member := range list.Members {
		emails = append(emails, member.Email)
	}
	return emails
}
