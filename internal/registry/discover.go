package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

const (
	listingLimit   = 4 << 20
	listingTimeout = 20 * time.Second
	// maxListed is the most models one listing adds, so a gateway that fronts every model there is
	// cannot fill the registry in one click.
	maxListed = 500
)

// Listed is a model a provider says it serves, with the price it gives when it gives one.
type Listed struct {
	Name  string
	Price *spend.Price
}

// Discover asks a provider of the organization's which models its key can use: GET /v1/models, which OpenAI's API and the
// gateways that copy it answer. The key is read here, on the server, and goes nowhere but the provider's own address, which
// is reached under the same rule as a model call.
func (r *Registry) Discover(ctx context.Context, name string) ([]Listed, error) {
	held := r.Document()
	i := indexOfProvider(held.Providers, name)
	if i < 0 || held.Providers[i].Retired {
		return nil, ErrNoSuch
	}
	provider := held.Providers[i]
	key, err := r.read(provider)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.Address+"/v1/models", nil)
	if err != nil {
		return nil, errors.New("that is not an address")
	}
	if provider.Auth == proxy.Anthropic {
		request.Header.Set("x-api-key", key)
		request.Header.Set("anthropic-version", proxy.AnthropicVersion)
	} else {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	answer, err := r.listingClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s did not answer", request.URL.Host)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s to the list of its models: check the key, and how it is sent", request.URL.Host, answer.Status)
	}
	return readListing(io.LimitReader(answer.Body, listingLimit), request.URL.Host)
}

// listingClient reaches a provider where a model call would: a public address, unless the operator allowed a private one. A
// redirect is not followed, since it would carry the key to wherever it points.
func (r *Registry) listingClient() *http.Client {
	if r.Rules().PrivateAddresses {
		return &http.Client{Timeout: listingTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	client := proxy.PublicClient(proxy.Resolver(r.resolve))
	client.Timeout = listingTimeout
	return client
}

// entry is one model of an OpenAI-shaped list. A gateway that prices what it lists says so in pricing.
type entry struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Pricing *struct {
		Input      float64  `json:"input_price_per_million"`
		Output     float64  `json:"output_price_per_million"`
		CacheRead  *float64 `json:"cache_read_price_per_million"`
		CacheWrite *float64 `json:"cache_write_price_per_million"`
		Unit       string   `json:"unit"`
	} `json:"pricing"`
}

// chat says whether an entry is a model an agent can talk to. A gateway lists what it can embed, draw and transcribe too.
func (e entry) chat() bool {
	return e.ID != "" && modelName.MatchString(e.ID) && (e.Type == "" || e.Type == "chat")
}

// price is what the entry costs per million tokens, or nil when it gives none or prices by another unit.
func (e entry) price() *spend.Price {
	p := e.Pricing
	if p == nil || (p.Unit != "" && p.Unit != "token") {
		return nil
	}
	price := &spend.Price{InputPerMillion: p.Input, OutputPerMillion: p.Output, CachedInputPerMillion: p.CacheRead, CacheWritePerMillion: p.CacheWrite}
	if checkPrice(price) != nil {
		return nil
	}
	return price
}

func readListing(body io.Reader, host string) ([]Listed, error) {
	var got struct {
		Data []entry `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&got); err != nil {
		return nil, fmt.Errorf("%s did not list its models the way OpenAI's API does", host)
	}
	var found []Listed
	for _, e := range got.Data {
		if !e.chat() {
			continue
		}
		found = append(found, Listed{Name: e.ID, Price: e.price()})
		if len(found) == maxListed {
			break
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%s lists no chat models for this key", host)
	}
	return found, nil
}

// Import adds every model a provider lists that the registry does not have, with the price the provider gave, off until an
// administrator turns each on. It says how many the provider listed and how many were new.
func (r *Registry) Import(ctx context.Context, name string) (listed, added int, err error) {
	found, err := r.Discover(ctx, name)
	if err != nil {
		return 0, 0, err
	}
	err = r.change(func(d *Document) error {
		if i := indexOfProvider(d.Providers, name); i < 0 || d.Providers[i].Retired {
			return ErrNoSuch
		}
		added = 0
		for _, model := range found {
			i := indexOfModel(d.Models, name, model.Name)
			switch {
			case i < 0:
				d.Models = append(d.Models, Model{Provider: name, Name: model.Name, Price: model.Price, Pending: true})
			case d.Models[i].Retired:
				d.Models[i] = Model{Provider: name, Name: model.Name, Price: model.Price, Pending: true}
			default:
				continue
			}
			added++
		}
		if added == 0 {
			return errors.New("every model it lists is here already")
		}
		return nil
	})
	return len(found), added, err
}
