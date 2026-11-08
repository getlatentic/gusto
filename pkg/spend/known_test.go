package spend

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestTheKnownPricesAreTheDemosPrices(t *testing.T) {
	demo, err := os.ReadFile("../../deploy/aws/openshell/ansible/roles/demo/files/spend-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy Policy
	if err := json.Unmarshal(demo, &policy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(Known().Prices, policy.Prices) {
		t.Error("pkg/spend/prices.json and the demo's spend-policy.json price different models or at different rates")
	}
}

func TestAKnownModelHasAPriceAndAnUnknownOneHasNone(t *testing.T) {
	known := Known()
	price, found := known.PriceOf("gemma", "google.gemma-4-26b-a4b")
	if !found || price.InputPerMillion != 0.13 || price.OutputPerMillion != 0.4 {
		t.Errorf("gemma 26b: %+v %v", price, found)
	}
	if _, found := known.PriceOf("gemma", "no-such-model"); found {
		t.Error("a model nobody priced has a price")
	}
	if len(known.Teams) != 0 || known.Organization != nil {
		t.Error("the known prices carry a cap")
	}
}
