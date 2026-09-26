package spend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturePolicy is a price table for tests. Its prices are made up, and are
// nobody's real price.
func fixturePolicy() Policy {
	return Policy{
		Prices: map[string]Price{"fixture-model": {InputPerMillion: 2, OutputPerMillion: 10}},
		Teams:  map[string]TeamCap{"platform": {USDPerMonth: 1}},
	}
}

func TestACallIsPricedFromTheTable(t *testing.T) {
	cost := fixturePolicy().Cost("fixture-model", &Usage{Input: 1_000_000, Output: 100_000})
	if cost == nil || *cost != 3 {
		t.Fatalf("cost %v", cost)
	}
}

func TestAModelWithNoPriceIsUnmeasuredRatherThanFree(t *testing.T) {
	policy := fixturePolicy()
	if cost := policy.Cost("another-model", &Usage{Input: 10, Output: 10}); cost != nil {
		t.Fatalf("an unpriced model cost %v", *cost)
	}
	if cost := policy.Cost("fixture-model", nil); cost != nil {
		t.Fatalf("an unmeasured answer cost %v", *cost)
	}
}

func TestThePolicyIsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.json")
	os.WriteFile(path, []byte(`{"prices":{"m":{"inputPerMillion":1,"outputPerMillion":2}},"teams":{"a":{"usdPerMonth":50}}}`), 0o644)
	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Priced("m") || policy.Teams["a"].USDPerMonth != 50 {
		t.Fatalf("policy %+v", policy)
	}
	if none, err := LoadPolicy(""); err != nil || none.Priced("m") {
		t.Fatalf("no file should be no policy, got %+v, %v", none, err)
	}
}

// A misspelt cap is no cap, silently, unless the file is refused.
func TestAPolicyWithAFieldThisBuildDoesNotKnowIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.json")
	os.WriteFile(path, []byte(`{"teams":{"a":{"usdPerMnth":50}}}`), 0o644)
	if _, err := LoadPolicy(path); err == nil || !strings.Contains(err.Error(), "usdPerMnth") {
		t.Fatalf("want the misspelt field named, got %v", err)
	}
}
