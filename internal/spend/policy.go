package spend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Policy is the operator's: what each model costs, and what each team may
// spend in a month. It is a file rather than code, because prices change more
// often than releases and belong to whoever holds the account.
type Policy struct {
	Prices map[string]Price   `json:"prices"`
	Teams  map[string]TeamCap `json:"teams"`
	// Organization is what every task of this server may spend together in a calendar month, UTC, whichever team they
	// count against. A demo has one, since an organization that lasts half an hour has nobody to set a team's cap.
	Organization *TeamCap `json:"organisation,omitempty"`
}

// TeamCap is what one team's tasks may spend together in a calendar month, UTC.
type TeamCap struct {
	USDPerMonth float64 `json:"usdPerMonth"`
}

// LoadPolicy reads the file at path. No path is no policy: every model is
// unpriced and no team is capped. A field the file names that this build does
// not know is refused, because a misspelt cap would otherwise be no cap.
func LoadPolicy(path string) (Policy, error) {
	if path == "" {
		return Policy{}, nil
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("the spend policy could not be read: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("%s is not a spend policy: %w", path, err)
	}
	return policy, nil
}

// Priced says whether a model has a price. A model with none costs an unknown
// amount, which is not the same as nothing.
func (p Policy) Priced(credential, model string) bool {
	_, found := p.price(credential, model)
	return found
}

// Cost is what this usage of this model cost when it was sent, or nil when
// either is unknown.
func (p Policy) Cost(credential, model string, usage *Usage, sent time.Time) *float64 {
	price, found := p.price(credential, model)
	if !found || usage == nil {
		return nil
	}
	cost := price.of(*usage, sent)
	return &cost
}

// price is found by the name a task asks for, the credential and the model,
// before the model alone: one model served through two credentials can be
// billed at two rates, and one served at one rate needs writing only once.
func (p Policy) price(credential, model string) (Price, bool) {
	if price, found := p.Prices[credential+"/"+model]; found && credential != "" {
		return price, true
	}
	price, found := p.Prices[model]
	return price, found
}

// organizationCap is the monthly cap on the whole server, if the policy sets one.
func (p Policy) organizationCap() (float64, bool) {
	if p.Organization == nil {
		return 0, false
	}
	return p.Organization.USDPerMonth, true
}

// teamCap is the monthly cap on a team, if the policy sets one.
func (p Policy) teamCap(team string) (float64, bool) {
	if team == "" {
		return 0, false
	}
	capped, found := p.Teams[team]
	return capped.USDPerMonth, found
}
