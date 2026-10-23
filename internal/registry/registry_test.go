package registry

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

var operators = Platform{
	Credentials: proxy.Credentials{"platform": {Name: "platform", Address: "https://models.operator.example", Key: "the-operators-key"}},
	Models:      []string{"platform/qwen"},
	Prices:      map[string]spend.Price{"platform/qwen": {InputPerMillion: 1, OutputPerMillion: 2}},
}

// dns is a resolver that answers from a table, and fails for a name it does not know.
func dns(table map[string]string) Resolver {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		if answer, found := table[host]; found {
			return []netip.Addr{netip.MustParseAddr(answer)}, nil
		}
		return nil, errors.New("no such host")
	}
}

func opened(t *testing.T, rules Rules) (*Registry, string) {
	t.Helper()
	dir := t.TempDir()
	r, err := Open(dir, operators, rules, false)
	if err != nil {
		t.Fatal(err)
	}
	r.SetResolver(dns(map[string]string{
		"api.theirs.example": "93.184.216.34", "inside.example": "10.0.4.7", "metadata.example": "169.254.169.254",
		"loop.example": "127.0.0.1", "mixed.example": "93.184.216.34",
	}))
	return r, dir
}

func theirs(name string) Give {
	return Give{Name: name, Address: "https://api.theirs.example/v1", Reference: "stored", Key: "sk-their-secret-key"}
}

func TestAnOrganizationAddsAProviderAndAModelAndTheProxyHoldsThemWithTheOperators(t *testing.T) {
	r, dir := opened(t, Rules{})
	var told []int64
	r.OnChange(func(e Effective) { told = append(told, e.Version) })
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Fatal(err)
	}
	price := &spend.Price{InputPerMillion: 0.5, OutputPerMillion: 1.5}
	if err := r.AddModel("acme", "big-1", price); err != nil {
		t.Fatal(err)
	}
	if held := r.Effective(); held.Credentials["acme"].Address != "" || slicesContain(held.Models, "acme/big-1") {
		t.Fatalf("what was added is in use before it was turned on: %+v", held)
	}
	for _, step := range [][2]string{{"acme", ""}, {"acme", "big-1"}} {
		if err := r.Enable(step[0], step[1]); err != nil {
			t.Fatal(err)
		}
	}
	got := r.Effective()
	if got.Version != 4 || !slicesEqual(told, []int64{1, 2, 3, 4}) {
		t.Errorf("each change is a version, and the listener hears of each: %d %v", got.Version, told)
	}
	credential := got.Credentials["acme"]
	if credential.Key != "sk-their-secret-key" || !credential.Public || credential.Address != "https://api.theirs.example/v1" {
		t.Errorf("%+v", credential)
	}
	if got.Credentials["platform"].Address == "" || got.Credentials["platform"].Public {
		t.Errorf("the operator's provider is there as it was, and is not held to an organization's limits: %+v", got.Credentials["platform"])
	}
	if !slicesEqual(got.Models, []string{"platform/qwen", "acme/big-1"}) {
		t.Errorf("the operator's models come first: %v", got.Models)
	}
	if got.Prices["acme/big-1"].OutputPerMillion != 1.5 || got.Prices["platform/qwen"].InputPerMillion != 1 {
		t.Errorf("%v", got.Prices)
	}
	for _, file := range []string{"registry.json", "keys/acme.json"} {
		info, err := os.Stat(filepath.Join(dir, file))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", file, info, err)
		}
	}
	text, _ := os.ReadFile(filepath.Join(dir, "registry.json"))
	if strings.Contains(string(text), "sk-their-secret-key") {
		t.Error("the key is in the registry's own file")
	}
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAProviderIsRefusedForWhatItIsCalledWhereItIsAndHowItsKeyIsGot(t *testing.T) {
	for name, tc := range map[string]struct {
		rules  Rules
		change func(*Give)
	}{
		"a name with a slash":            {Rules{}, func(g *Give) { g.Name = "a/b" }},
		"a name with capitals":           {Rules{}, func(g *Give) { g.Name = "Acme" }},
		"no name":                        {Rules{}, func(g *Give) { g.Name = "" }},
		"the operator's name":            {Rules{}, func(g *Give) { g.Name = "platform" }},
		"plain http":                     {Rules{}, func(g *Give) { g.Address = "http://api.theirs.example" }},
		"a web address with a password":  {Rules{}, func(g *Give) { g.Address = "https://u:p@api.theirs.example" }},
		"a query":                        {Rules{}, func(g *Give) { g.Address = "https://api.theirs.example/?a=1" }},
		"a name that is not there":       {Rules{}, func(g *Give) { g.Address = "https://nowhere.example" }},
		"localhost":                      {Rules{}, func(g *Give) { g.Address = "https://localhost" }},
		"a name on a private network":    {Rules{}, func(g *Give) { g.Address = "https://inside.example" }},
		"the metadata service by name":   {Rules{}, func(g *Give) { g.Address = "https://metadata.example" }},
		"the metadata service by number": {Rules{}, func(g *Give) { g.Address = "https://169.254.169.254" }},
		"this machine by number":         {Rules{}, func(g *Give) { g.Address = "https://127.0.0.1:9101" }},
		"this machine by name":           {Rules{}, func(g *Give) { g.Address = "https://loop.example" }},
		"an address on a private range":  {Rules{}, func(g *Give) { g.Address = "https://[fd00::1]" }},
		"an address with nothing":        {Rules{}, func(g *Give) { g.Address = "" }},
		"an unknown auth":                {Rules{}, func(g *Give) { g.Auth = "basic" }},
		"no key kept":                    {Rules{}, func(g *Give) { g.Key = "" }},
		"a key with a newline":           {Rules{}, func(g *Give) { g.Key = "a\nb" }},
		"a key that is typed and named":  {Rules{KeySources: []string{"file"}}, func(g *Give) { g.Reference = "file:/etc/key" }},
		"a file where none is allowed":   {Rules{}, func(g *Give) { g.Reference, g.Key = "file:/etc/passwd", "" }},
		"a parameter where none is":      {Rules{}, func(g *Give) { g.Reference, g.Key = "ssm:/gusto/poc/model-key", "" }},
		"a secret where none is":         {Rules{}, func(g *Give) { g.Reference, g.Key = "secretsmanager:x", "" }},
		"a reference of no kind":         {Rules{KeySources: []string{"file"}}, func(g *Give) { g.Reference, g.Key = "env:HOME", "" }},
		"a relative file":                {Rules{KeySources: []string{"file"}}, func(g *Give) { g.Reference, g.Key = "file:key", "" }},
		"a parameter that is an option":  {Rules{KeySources: []string{"ssm"}}, func(g *Give) { g.Reference, g.Key = "ssm:-x", "" }},
		"a parameter with a space":       {Rules{KeySources: []string{"ssm"}}, func(g *Give) { g.Reference, g.Key = "ssm:a b", "" }},
	} {
		t.Run(name, func(t *testing.T) {
			r, dir := opened(t, tc.rules)
			give := theirs("acme")
			tc.change(&give)
			if err := r.AddProvider(give); err == nil {
				t.Fatalf("it was added: %+v", r.Document())
			}
			if r.Document().Version != 0 || len(r.Document().Providers) != 0 {
				t.Errorf("a refused change left a mark: %+v", r.Document())
			}
			if _, err := os.Stat(filepath.Join(dir, "registry.json")); err == nil {
				t.Error("a file was written")
			}
			if entries, _ := os.ReadDir(filepath.Join(dir, "keys")); len(entries) != 0 {
				t.Errorf("a key was kept for a provider that was refused: %v", entries)
			}
		})
	}
}

func TestAnOperatorWhoRunsAModelInsideTheirNetworkMayNameItAndAnAdminOfTheirsIsHeldToNothingElse(t *testing.T) {
	r, _ := opened(t, Rules{PrivateAddresses: true})
	give := theirs("inside")
	give.Address = "http://10.0.4.7:11434"
	if err := r.AddProvider(give); err != nil {
		t.Fatal(err)
	}
	if r.Effective().Credentials["inside"].Public {
		t.Error("a deployment that allows private addresses does not hold its providers to public ones")
	}
}

func TestADuplicateIsRefusedAndARetiredProviderCanBeAddedAgainAndItsKeyIsGone(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	if err := r.AddProvider(theirs("acme")); err == nil {
		t.Fatal("two providers of one name")
	}
	r.AddModel("acme", "m", nil)
	r.Enable("acme", "m")
	if err := r.RetireProvider("acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "acme.json")); err == nil {
		t.Error("the key of a retired provider is kept")
	}
	got := r.Effective()
	if _, held := got.Credentials["acme"]; held || slicesContain(got.Models, "acme/m") {
		t.Errorf("a retired provider and its models are not in use: %v %v", got.Credentials, got.Models)
	}
	if err := r.RetireProvider("acme"); !errors.Is(err, ErrNoSuch) {
		t.Errorf("%v", err)
	}
	if err := r.AddModel("acme", "n", nil); !errors.Is(err, ErrNoSuch) {
		t.Errorf("a model cannot be added to a retired provider: %v", err)
	}
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Errorf("a retired provider's name is free: %v", err)
	}
	if slicesContain(r.Effective().Models, "acme/m") {
		t.Error("its old model is back with it")
	}
}

func slicesContain(list []string, want string) bool {
	for _, each := range list {
		if each == want {
			return true
		}
	}
	return false
}

func TestModelsArePricedChangedRetiredAndBroughtBack(t *testing.T) {
	r, _ := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	if err := r.AddModel("acme", "m", nil); err != nil {
		t.Fatal(err)
	}
	r.Enable("acme", "m")
	if _, priced := r.Effective().Prices["acme/m"]; priced {
		t.Error("a model with no price is unpriced, which is not free")
	}
	if err := r.AddModel("acme", "m", nil); err == nil {
		t.Error("a model twice")
	}
	if err := r.SetPrice("acme", "m", &spend.Price{InputPerMillion: 3, OutputPerMillion: 4}); err != nil {
		t.Fatal(err)
	}
	if r.Effective().Prices["acme/m"].OutputPerMillion != 4 {
		t.Error("the price is not used")
	}
	if err := r.RetireModel("acme", "m"); err != nil {
		t.Fatal(err)
	}
	if got := r.Effective(); slicesContain(got.Models, "acme/m") {
		t.Errorf("%v", got.Models)
	} else if _, priced := got.Prices["acme/m"]; priced {
		t.Error("a retired model is still priced")
	}
	if err := r.SetPrice("acme", "m", nil); !errors.Is(err, ErrNoSuch) {
		t.Errorf("%v", err)
	}
	if err := r.AddModel("acme", "m", nil); err != nil || slicesContain(r.Effective().Models, "acme/m") {
		t.Errorf("a retired model comes back, and waits to be turned on: %v", err)
	}
	r.Enable("acme", "m")
	if !slicesContain(r.Effective().Models, "acme/m") {
		t.Error("a model that was brought back and turned on is not offered")
	}
	if err := r.RetireModel("acme", "nothing"); !errors.Is(err, ErrNoSuch) {
		t.Errorf("%v", err)
	}
}

func TestAPriceOrAModelNameThatIsWrongIsRefused(t *testing.T) {
	r, _ := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	neg, huge := -1.0, 20000.0
	for name, price := range map[string]*spend.Price{
		"below nothing":      {InputPerMillion: neg},
		"above any provider": {OutputPerMillion: huge},
		"a cached rate":      {InputPerMillion: 1, CachedInputPerMillion: &neg},
		"a peak":             {InputPerMillion: 1, Peak: &spend.Peak{Multiplier: 2}},
	} {
		if err := r.AddModel("acme", "m", price); err == nil {
			t.Errorf("%s: a price was accepted", name)
		}
	}
	for _, name := range []string{"", " ", "-a", "a b", "a;b", strings.Repeat("a", 129), "../x"} {
		if err := r.AddModel("acme", name, nil); err == nil {
			t.Errorf("%q was accepted as a model", name)
		}
	}
	for _, name := range []string{"qwen3-vl-235b", "anthropic.claude-3:0", "org/model-7b"} {
		if err := r.AddModel("acme", name, nil); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	if r.Document().Version != 5 {
		t.Errorf("only the changes that were made are versions: %d", r.Document().Version)
	}
}

func TestARegistryIsReadBackAsItWasKeptAndOneThatCannotBeReadIsAnErrorAndNotNothing(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	r.AddModel("acme", "m", &spend.Price{InputPerMillion: 1})
	r.Enable("acme", "m")
	again, err := Open(dir, operators, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Effective(); got.Version != 4 || got.Credentials["acme"].Key != "sk-their-secret-key" || !slicesContain(got.Models, "acme/m") {
		t.Errorf("%+v", got)
	}
	os.WriteFile(filepath.Join(dir, "registry.json"), []byte("{not json"), 0o600)
	if _, err := Open(dir, operators, Rules{}, false); err == nil {
		t.Error("a file that is not a registry was read as an empty one")
	}
}

func TestAProviderWhoseKeyCannotBeReadIsLeftOutWithTheReasonAndItsModelsWithIt(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	r.AddModel("acme", "m", nil)
	r.Enable("acme", "m")
	os.Remove(filepath.Join(dir, "keys", "acme.json"))
	got := r.Effective()
	if _, held := got.Credentials["acme"]; held || slicesContain(got.Models, "acme/m") {
		t.Errorf("a provider with no key is offered: %v %v", got.Credentials, got.Models)
	}
	if !strings.Contains(got.Problems["acme"], "no key") {
		t.Errorf("%v", got.Problems)
	}
	if !slicesContain(got.Models, "platform/qwen") {
		t.Error("the operator's models are still there")
	}
}

func TestAKeyIsReplacedWithoutChangingAnythingElse(t *testing.T) {
	r, _ := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	if err := r.ReplaceKey("acme", "sk-the-new-one"); err != nil {
		t.Fatal(err)
	}
	got := r.Effective()
	if got.Credentials["acme"].Key != "sk-the-new-one" || got.Version != 3 {
		t.Errorf("%+v", got)
	}
	if err := r.ReplaceKey("platform", "x"); !errors.Is(err, ErrNoSuch) {
		t.Errorf("the operator's key is not an organization's to replace: %v", err)
	}
	if err := r.ReplaceKey("acme", ""); err == nil {
		t.Error("no key")
	}
}

func TestAnOrganizationCannotAddAProviderBesideAnUnnamedOne(t *testing.T) {
	legacy := Platform{Credentials: proxy.Credentials{"": {Address: "https://models.operator.example", Key: "k"}}, Models: []string{"qwen"}}
	r, err := Open(t.TempDir(), legacy, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	r.SetResolver(dns(map[string]string{"api.theirs.example": "93.184.216.34"}))
	if err := r.AddProvider(theirs("acme")); err == nil {
		t.Error("a second provider makes the unnamed one serve nothing, since it serves only while it is alone")
	}
}

func TestReadingKeysFromElsewhereIsDoneOnlyWhereTheOperatorAllowedItAndWithNoShell(t *testing.T) {
	var ran [][]string
	run := func(name string, args ...string) ([]byte, error) {
		ran = append(ran, append([]string{name}, args...))
		return []byte(" sk-from-parameter-store\n"), nil
	}
	r, dir := opened(t, Rules{KeySources: []string{"ssm", "file"}})
	r.run = run
	if err := r.AddProvider(Give{Name: "acme", Address: "https://api.theirs.example", Reference: "ssm:/acme/model-key"}); err != nil {
		t.Fatal(err)
	}
	r.Enable("acme", "")
	if got := r.Effective().Credentials["acme"].Key; got != "sk-from-parameter-store" {
		t.Errorf("%q", got)
	}
	want := []string{"aws", "ssm", "get-parameter", "--with-decryption", "--name", "/acme/model-key", "--query", "Parameter.Value", "--output", "text"}
	if len(ran) != 1 || strings.Join(ran[0], " ") != strings.Join(want, " ") {
		t.Errorf("%v", ran)
	}
	keyfile := filepath.Join(t.TempDir(), "key")
	os.WriteFile(keyfile, []byte("sk-from-file\n"), 0o600)
	if err := r.AddProvider(Give{Name: "filed", Address: "https://api.theirs.example", Reference: "file:" + keyfile}); err != nil {
		t.Fatal(err)
	}
	r.Enable("filed", "")
	if got := r.Effective().Credentials["filed"].Key; got != "sk-from-file" {
		t.Errorf("%q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "acme.json")); err == nil {
		t.Error("a key that is read from elsewhere was copied here")
	}
	// A deployment that has stopped allowing a kind stops reading it, whatever was kept.
	stricter, err := Open(dir, operators, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	stricter.run = run
	if _, held := stricter.Effective().Credentials["acme"]; held {
		t.Error("a kind that is no longer allowed is still read")
	}
}

func TestTheViewSaysEveryProviderAndWhereItsKeyIsAndNeverTheKey(t *testing.T) {
	r, _ := opened(t, Rules{KeySources: []string{"file"}})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	r.AddModel("acme", "m", &spend.Price{InputPerMillion: 1, OutputPerMillion: 2})
	r.Enable("acme", "m")
	r.AddModel("acme", "gone", nil)
	r.Enable("acme", "gone")
	r.RetireModel("acme", "gone")
	view := r.View()
	if view.Version != 7 || view.Private || view.Unnamed || !slicesEqual(view.Kinds, []string{"stored", "file"}) {
		t.Errorf("%+v", view)
	}
	if len(view.Providers) != 2 || view.Providers[0].Name != "platform" || !view.Providers[0].Operator || view.Providers[0].Key != "the operator's" {
		t.Fatalf("the operator's first: %+v", view.Providers)
	}
	if got := view.Providers[0].Models; len(got) != 1 || got[0].Full != "platform/qwen" || got[0].Price == nil || got[0].Price.OutputPerMillion != 2 {
		t.Errorf("%+v", got)
	}
	acme := view.Providers[1]
	if acme.Operator || acme.Key != "stored" || len(acme.Models) != 2 || !acme.Models[1].Retired || acme.Models[0].Price == nil {
		t.Errorf("%+v", acme)
	}
	if strings.Contains(fmt.Sprintf("%+v", view), "sk-their-secret-key") || strings.Contains(fmt.Sprintf("%+v", view), "the-operators-key") {
		t.Error("a key is in the view")
	}
}

func TestARefusedSecondProviderOfTheSameNameDoesNotTakeTheFirstOnesKey(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	other := theirs("acme")
	other.Key = "sk-somebody-elses-key"
	if err := r.AddProvider(other); err == nil {
		t.Fatal("two providers of one name")
	}
	if got := r.Effective().Credentials["acme"].Key; got != "sk-their-secret-key" {
		t.Errorf("the first provider's key was replaced or lost: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "keys", "acme.json")); err != nil {
		t.Errorf("the first provider's key file is gone: %v", err)
	}
}

func TestAnAdminWhereTheRegistryIsEditableChoosesWhatIsAllowedAndItTakesEffectAtOnce(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir, operators, Rules{}, true)
	if err != nil {
		t.Fatal(err)
	}
	r.SetResolver(dns(map[string]string{"api.theirs.example": "93.184.216.34", "inside.example": "10.0.4.7"}))
	inside := Give{Name: "inside", Address: "http://inside.example:11434", Reference: "stored", Key: "k"}
	if err := r.AddProvider(inside); err == nil {
		t.Fatal("a private address is refused until the admin allows it")
	}
	if err := r.SetPolicy(Policy{PrivateAddresses: true, KeySources: []string{"ssm", "file", "ssm"}}); err != nil {
		t.Fatal(err)
	}
	if got := r.Rules(); !got.PrivateAddresses || len(got.KeySources) != 2 || got.KeySources[0] != "file" {
		t.Errorf("%+v", got)
	}
	if err := r.AddProvider(inside); err != nil {
		t.Fatalf("the choice applies to the next change: %v", err)
	}
	if r.Effective().Credentials["inside"].Public {
		t.Error("a provider at a private address is not held to public ones once that is allowed")
	}
	if view := r.View(); !view.Editable || !view.Private || len(view.Kinds) != 3 {
		t.Errorf("%+v", view)
	}
	again, _ := Open(dir, operators, Rules{}, true)
	if !again.Rules().PrivateAddresses || again.Document().Version != 2 {
		t.Errorf("the choice is kept: %+v %d", again.Rules(), again.Document().Version)
	}
	if err := r.SetPolicy(Policy{KeySources: []string{"env"}}); err == nil {
		t.Error("a kind that is not one was accepted")
	}
}

func TestWhereTheOperatorFixesThePolicyAnAdminCannotChangeItAndAStoredChoiceIsIgnored(t *testing.T) {
	dir := t.TempDir()
	editable, _ := Open(dir, operators, Rules{}, true)
	editable.SetPolicy(Policy{PrivateAddresses: true, KeySources: []string{"file"}})
	fixed, err := Open(dir, operators, Rules{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fixed.Rules(); got.PrivateAddresses || len(got.KeySources) != 0 {
		t.Errorf("a file left by an editable deployment loosens a fixed one: %+v", got)
	}
	if err := fixed.SetPolicy(Policy{PrivateAddresses: true}); !errors.Is(err, ErrFixed) {
		t.Errorf("%v", err)
	}
	if fixed.View().Editable {
		t.Error("the page offers a choice that is not there")
	}
}

func TestAPersonIsLimitedToSomeModelsAndNobodyElseIs(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	r.AddModel("acme", "big", nil)
	r.Enable("acme", "big")
	r.AddModel("acme", "small", nil)
	r.Enable("acme", "small")
	if _, limited := r.AllowedFor("ada@acme.com"); limited {
		t.Fatal("a person nobody said anything about is limited")
	}
	if err := r.SetAccess(" Ada@Acme.com ", []string{"acme/small", "platform/qwen", "acme/small"}); err != nil {
		t.Fatal(err)
	}
	got, limited := r.AllowedFor("ADA@acme.com")
	if !limited || len(got) != 2 || got[0] != "acme/small" || got[1] != "platform/qwen" {
		t.Errorf("an address is one person whatever its case, and a model is listed once: %v %v", got, limited)
	}
	if _, limited := r.AllowedFor("bob@acme.com"); limited {
		t.Error("another person was limited")
	}
	again, _ := Open(dir, operators, Rules{}, false)
	if list := again.Limits(); len(list) != 1 || list[0].Email != "ada@acme.com" {
		t.Errorf("kept: %+v", list)
	}
	if view := r.View(); len(view.Limits) != 1 || len(view.Available) != 3 {
		t.Errorf("%+v", view)
	}
	for name, call := range map[string]func() error{
		"a model there is not":         func() error { return r.SetAccess("ada@acme.com", []string{"acme/nothing"}) },
		"not an address":               func() error { return r.SetAccess("ada", []string{"acme/big"}) },
		"a name and address":           func() error { return r.SetAccess("Ada <ada@acme.com>", []string{"acme/big"}) },
		"a limit removed that was not": func() error { return r.SetAccess("bob@acme.com", nil) },
	} {
		if call() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	before := r.Document().Version
	if err := r.SetAccess("ada@acme.com", nil); err != nil {
		t.Fatal(err)
	}
	if _, limited := r.AllowedFor("ada@acme.com"); limited || r.Document().Version != before+1 {
		t.Error("no models is no limit")
	}
	r.RetireModel("acme", "small")
	r.SetAccess("cy@acme.com", []string{"acme/big"})
	if err := r.SetAccess("cy@acme.com", []string{"acme/small"}); err == nil {
		t.Error("a retired model can be given to somebody")
	}
}

func TestWhatIsAddedIsOffUntilAnAdministratorTurnsItOnAndCanBeTurnedOffAgain(t *testing.T) {
	r, _ := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.AddModel("acme", "m", &spend.Price{InputPerMillion: 1, OutputPerMillion: 2})
	view := r.View()
	if !view.Providers[1].Pending || !view.Providers[1].Models[0].Pending {
		t.Fatalf("the view does not say what is waiting: %+v", view.Providers[1])
	}
	if got := r.Effective(); got.Credentials["acme"].Address != "" || slicesContain(got.Models, "acme/m") || len(got.Problems) != 0 {
		t.Fatalf("something that was only added is in use: %+v", got)
	}
	if err := r.Enable("acme", "m"); err != nil {
		t.Fatal(err)
	}
	if slicesContain(r.Effective().Models, "acme/m") {
		t.Error("a model is offered while its provider is off")
	}
	r.Enable("acme", "")
	if got := r.Effective(); !slicesContain(got.Models, "acme/m") || got.Credentials["acme"].Key == "" {
		t.Errorf("turned on, and not in use: %+v", got)
	}
	r.Disable("acme", "m")
	if slicesContain(r.Effective().Models, "acme/m") {
		t.Error("a model that was turned off is still offered")
	}
	r.Disable("acme", "")
	if _, held := r.Effective().Credentials["acme"]; held {
		t.Error("a provider that was turned off is still held")
	}
	if err := r.Enable("acme", "nothing"); !errors.Is(err, ErrNoSuch) {
		t.Errorf("err = %v", err)
	}
	if err := r.Enable("nobody", ""); !errors.Is(err, ErrNoSuch) {
		t.Errorf("err = %v", err)
	}
}

func TestARegistryKeptBeforeThereWasAnOffSwitchHasEverythingOn(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"version":3,"providers":[{"name":"acme","address":"https://api.theirs.example/v1","key":"file:/nonexistent"}],"models":[{"provider":"acme","name":"m"}]}`), 0o600)
	keyfile := filepath.Join(t.TempDir(), "key")
	os.WriteFile(keyfile, []byte("sk-x\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "registry.json"), []byte(`{"version":3,"providers":[{"name":"acme","address":"https://api.theirs.example/v1","key":"file:`+keyfile+`"}],"models":[{"provider":"acme","name":"m"}]}`), 0o600)
	r, err := Open(dir, operators, Rules{KeySources: []string{"file"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Effective(); !slicesContain(got.Models, "acme/m") || got.Credentials["acme"].Key != "sk-x" {
		t.Errorf("a provider and a model that were on before are off after an upgrade: %+v", got)
	}
}

func TestTheRegistryKeepsWhatItWasAndPutsItBackAsAChangeOfItsOwn(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	r.Enable("acme", "")
	r.AddModel("acme", "m", &spend.Price{InputPerMillion: 1})
	r.Enable("acme", "m")
	before := r.Document().Version
	r.SetPrice("acme", "m", &spend.Price{InputPerMillion: 99})
	r.RetireModel("acme", "m")
	steps := r.Steps()
	if len(steps) != int(before)+1 || steps[0].Version != before+1 || strings.Join(steps[0].Then, ";") != "retired model acme/m" || strings.Join(steps[1].Then, ";") != "changed the price of acme/m" {
		t.Fatalf("steps = %+v", steps)
	}
	var told []Effective
	r.OnChange(func(e Effective) { told = append(told, e) })
	undone, err := r.Restore(before)
	if err != nil || strings.Join(undone, ";") != "brought back model acme/m;changed the price of acme/m" {
		t.Fatalf("undone = %v, %v", undone, err)
	}
	got := r.Effective()
	if !slicesContain(got.Models, "acme/m") || got.Prices["acme/m"].InputPerMillion != 1 || got.Version != before+3 || len(told) != 1 {
		t.Errorf("after the restore: %+v (told %d)", got, len(told))
	}
	if _, err := r.Restore(before + 100); err == nil {
		t.Error("a version the registry never had was restored")
	}
	again, err := Open(dir, operators, Rules{}, false)
	if err != nil || len(again.Steps()) != len(r.Steps()) {
		t.Errorf("what was kept is not there after a restart: %v", err)
	}
	if text, _ := os.ReadFile(filepath.Join(dir, "history.json")); strings.Contains(string(text), "sk-their-secret-key") {
		t.Error("the history holds a key")
	}
}

func TestTheHistoryIsBoundedAndADamagedOneIsAnErrorAndNotNothing(t *testing.T) {
	r, dir := opened(t, Rules{})
	r.AddProvider(theirs("acme"))
	for range 30 {
		r.Enable("acme", "")
		r.Disable("acme", "")
	}
	if got := len(r.Steps()); got != mostRevisions {
		t.Errorf("%d states are kept, and %d is the most", got, mostRevisions)
	}
	os.WriteFile(filepath.Join(dir, "history.json"), []byte("{not json"), 0o600)
	if _, err := Open(dir, operators, Rules{}, false); err == nil {
		t.Error("a history that is not one was read as none")
	}
}
