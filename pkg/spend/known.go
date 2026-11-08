package spend

import (
	"bytes"
	_ "embed"
	"encoding/json"
)

//go:embed prices.json
var knownPrices []byte

// Known is the prices of the models gusto's own deployments use, with no team caps: the figures another program needs to cost a
// call on those models without keeping a copy. A server's own policy file, read with LoadPolicy, is what it charges against.
func Known() Policy {
	decoder := json.NewDecoder(bytes.NewReader(knownPrices))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		panic("pkg/spend/prices.json is not a spend policy: " + err.Error())
	}
	return policy
}

// PriceOf is the price of a model asked for as a credential and a model, or false when it has none.
func (p Policy) PriceOf(credential, model string) (Price, bool) {
	return p.price(credential, model)
}
