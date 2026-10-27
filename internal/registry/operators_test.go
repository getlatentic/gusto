package registry

import (
	"slices"
	"testing"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// The demo prices DeepSeek's models by their name alone, as the ledger reads them, and lists a model whose credential is gone.
func TestTheOperatorsModelsArePricedAsTheLedgerPricesThemAndOneWithNoCredentialIsNotOffered(t *testing.T) {
	platform := Platform{
		Credentials: proxy.Credentials{"deepseek": {Name: "deepseek", Address: "https://api.deepseek.com"}},
		Models:      []string{"deepseek/deepseek-flash", "anthropic/deepseek-flash"},
		Prices:      map[string]spend.Price{"deepseek-flash": {InputPerMillion: 0.15, OutputPerMillion: 0.6}},
	}
	r, err := Open(t.TempDir(), platform, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Effective().Models; !slices.Equal(got, []string{"deepseek/deepseek-flash"}) {
		t.Errorf("offered %v", got)
	}
	view := r.View()
	if len(view.Providers) != 1 || len(view.Providers[0].Models) != 1 {
		t.Fatalf("the page shows %+v", view.Providers)
	}
	if price := view.Providers[0].Models[0].Price; price == nil || price.InputPerMillion != 0.15 {
		t.Errorf("deepseek-flash is shown with price %+v", price)
	}
	if !slices.Equal(view.Available, []string{"deepseek/deepseek-flash"}) {
		t.Errorf("a person may be limited to %v", view.Available)
	}
}

func TestADeploymentThatHoldsNoCredentialsKeepsEveryModel(t *testing.T) {
	r, err := Open(t.TempDir(), Platform{Models: []string{"qwen"}}, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Effective().Models; !slices.Equal(got, []string{"qwen"}) {
		t.Errorf("offered %v", got)
	}
}
