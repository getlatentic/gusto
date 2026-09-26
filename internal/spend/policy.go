package spend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Policy is the operator's: what each model costs, and what each team may
// spend in a month. It is a file rather than code, because prices change more
// often than releases and belong to whoever holds the account.
type Policy struct {
	Prices map[string]Price   `json:"prices"`
	Teams  map[string]TeamCap `json:"teams"`
}

// Price is what a provider charges for a million tokens of each kind, in US
// dollars.
type Price struct {
	InputPerMillion  float64 `json:"inputPerMillion"`
	OutputPerMillion float64 `json:"outputPerMillion"`
}

// TeamCap is what one team's jobs may spend together in a calendar month, UTC.
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
func (p Policy) Priced(model string) bool {
	_, found := p.Prices[model]
	return found
}

// Cost is what this usage of this model cost, or nil when either is unknown.
func (p Policy) Cost(model string, usage *Usage) *float64 {
	price, found := p.Prices[model]
	if !found || usage == nil {
		return nil
	}
	cost := float64(usage.Input)*price.InputPerMillion/1e6 + float64(usage.Output)*price.OutputPerMillion/1e6
	return &cost
}

// teamCap is the monthly cap on a team, if the policy sets one.
func (p Policy) teamCap(team string) (float64, bool) {
	if team == "" {
		return 0, false
	}
	capped, found := p.Teams[team]
	return capped.USDPerMonth, found
}
