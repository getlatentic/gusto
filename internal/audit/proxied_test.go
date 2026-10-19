package audit_test

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/audit"
)

// A cap refusal never left the box and a request to another host is not a
// model call, so only the two model calls are counted as spend. The others
// have null costs, which must not make the model's cost unknown.
func TestSpendCountsOnlyTheModelCallsThatLeft(t *testing.T) {
	export := exported(t, ran(t), audit.Options{})
	matchesTheSchema(t, export)
	record := summary(t, export)
	for _, tc := range []struct {
		path []string
		want any
	}{
		{[]string{"spend", "proxyCalls"}, 2.0},
		{[]string{"spend", "proxyBytes"}, 2000.0},
		{[]string{"spend", "proxyTokens"}, 180.0},
		{[]string{"spend", "proxyUsd"}, 0.75},
		{[]string{"spend", "refusedCalls"}, 1.0},
		{[]string{"spend", "costUsd"}, audit.NotRecorded},
	} {
		if got := at(record, tc.path...); got != tc.want {
			t.Errorf("%s: got %#v, want %#v", strings.Join(tc.path, "."), got, tc.want)
		}
	}
	if reached := at(record, "outbound", "reached"); !sameStrings(reached, "registry.npmjs.org:443") {
		t.Errorf("outbound.reached: got %#v", reached)
	}
	if denied := at(record, "outbound", "denied"); !sameStrings(denied, "exfil.example:443") {
		t.Errorf("outbound.denied: got %#v", denied)
	}
}

// One call the proxy could not price makes the total unknown. A partial sum
// would read as the whole cost.
func TestAnUnmeasuredCallIsNotMeasuredRatherThanFree(t *testing.T) {
	unpriced := `{"path":"/v1/chat/completions","status":200,"bytes":10,"tookMs":5,"atUnix":106,"model":"unpriced","inputTokens":null,"outputTokens":null,"usd":null}`
	export := exported(t, ranWith(t, firstCall, unpriced), audit.Options{})
	matchesTheSchema(t, export)
	record := summary(t, export)
	if got := at(record, "spend", "proxyCalls"); got != 2.0 {
		t.Errorf("proxyCalls: got %#v", got)
	}
	for _, name := range []string{"proxyUsd", "proxyTokens"} {
		if got := at(record, "spend", name); got != audit.NotMeasured {
			t.Errorf("%s: got %#v, want %q", name, got, audit.NotMeasured)
		}
	}
}

// A job a cap stopped before anything left spent a measured nothing, which
// is zero and not a word.
func TestAJobRefusedBeforeItsFirstCallSpentZero(t *testing.T) {
	export := exported(t, ranWith(t, refusedCall), audit.Options{})
	matchesTheSchema(t, export)
	record := summary(t, export)
	for _, tc := range []struct {
		name string
		want any
	}{
		{"proxyCalls", 0.0},
		{"proxyUsd", 0.0},
		{"proxyTokens", 0.0},
		{"refusedCalls", 1.0},
	} {
		if got := at(record, "spend", tc.name); got != tc.want {
			t.Errorf("%s: got %#v, want %#v", tc.name, got, tc.want)
		}
	}
	if credentials := at(record, "model", "credentials"); !sameStrings(credentials) {
		t.Errorf("a refused call adds no credential: got %#v", credentials)
	}
}

// A cost the policy hid is redacted, not unmeasured: the proxy knew it.
func TestARedactedCostSaysSo(t *testing.T) {
	policy := audit.Policy{Redact: audit.Redact{Fields: []string{"usd", "host"}}}
	export := exported(t, ran(t), audit.Options{Policy: policy})
	if problems := verified(t, export).Problems; len(problems) != 0 {
		t.Fatalf("a redacted export should verify: %v", problems)
	}
	matchesTheSchema(t, export)
	record := summary(t, export)
	if got := at(record, "spend", "proxyUsd"); got != audit.Redacted {
		t.Errorf("proxyUsd: got %#v, want %q", got, audit.Redacted)
	}
	if got := at(record, "spend", "proxyCalls"); got != 2.0 {
		t.Errorf("a redacted host still is not a model call: proxyCalls %#v", got)
	}
}

// With the proxy record dropped there is nothing to count, and every count
// says why.
func TestADroppedProxyRecordIsRedacted(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: audit.Policy{Drop: []string{"out/proxy.jsonl"}}})
	matchesTheSchema(t, export)
	record := summary(t, export)
	for _, path := range [][]string{
		{"spend", "proxyCalls"}, {"spend", "proxyUsd"}, {"spend", "refusedCalls"},
		{"outbound", "reached"}, {"outbound", "denied"},
	} {
		if got := at(record, path...); got != audit.Redacted {
			t.Errorf("%s: got %#v, want %q", strings.Join(path, "."), got, audit.Redacted)
		}
	}
}

// Every call is written with the version of the organization's registry it was sent under, and the
// record says which versions a job's calls were sent under, so a change of models or prices is never
// mistaken for a change of what the job did.
func TestTheRecordSaysWhichVersionsOfTheModelRegistryTheCallsWereSentUnder(t *testing.T) {
	calls := []string{
		`{"path":"/v1/chat/completions","status":200,"tookMs":5,"atUnix":100,"credential":"acme","registry":7,"model":"m","inputTokens":10,"outputTokens":5,"usd":0.1}`,
		`{"path":"/v1/chat/completions","status":200,"tookMs":5,"atUnix":101,"credential":"acme","registry":9,"model":"m","inputTokens":10,"outputTokens":5,"usd":0.1}`,
		`{"path":"/v1/chat/completions","status":200,"tookMs":5,"atUnix":102,"credential":"acme","registry":7,"model":"m","inputTokens":10,"outputTokens":5,"usd":0.1}`,
	}
	export := exported(t, ranWith(t, calls...), audit.Options{})
	matchesTheSchema(t, export)
	versions, _ := at(summary(t, export), "model", "registry").([]any)
	if len(versions) != 2 || versions[0] != 7.0 || versions[1] != 9.0 {
		t.Errorf("the versions, once each and in order: %#v", versions)
	}
	bare := exported(t, ranWith(t, refusedCall), audit.Options{})
	if got, _ := at(summary(t, bare), "model", "registry").([]any); len(got) != 0 {
		t.Errorf("a deployment with no registry lists none: %#v", got)
	}
}
