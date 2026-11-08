package boot

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/pkg/spend"
)

// The models a demo offers each have a price, so that what a demo spends is not shown as unknown.
func TestEveryModelTheDemoOffersHasAPriceInTheDemosSpendPolicy(t *testing.T) {
	policy, err := spend.LoadPolicy("../../ansible/roles/demo/files/spend-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := os.ReadFile("../../ansible/roles/demo/defaults/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^demo_models: (.+)$`).FindSubmatch(defaults)
	if found == nil {
		t.Fatal("the demo names no models")
	}
	for _, offered := range strings.Split(string(found[1]), ",") {
		_, model, _ := strings.Cut(offered, "/")
		if price, priced := policy.Prices[model]; !priced || price.InputPerMillion <= 0 || price.OutputPerMillion <= 0 {
			t.Errorf("%s has no price", offered)
		}
	}
}

// A demo is an organization that lasts half an hour, so what its agents may spend together is capped as a whole: by the credit it
// starts with, which a key the visitor adds does not use.
func TestADemoSpendsNoMoreThanADollarInAll(t *testing.T) {
	if credit := cloud.DefaultDemoLimits().CreditUSD; credit <= 0 || credit > 1 {
		t.Errorf("a demo's credit is not a dollar or less: %v", credit)
	}
}
