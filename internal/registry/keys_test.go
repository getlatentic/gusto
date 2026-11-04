package registry

import (
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/spend"
)

// heldKeys keeps keys in files, and holds a Keep of one key, or a Forget, until it is let go.
type heldKeys struct {
	FileKeys
	holdKey    string
	holdForget bool
	reached    chan struct{}
	release    chan struct{}
}

func holding(dir string) *heldKeys {
	return &heldKeys{FileKeys: FileKeys(dir), reached: make(chan struct{}, 1), release: make(chan struct{})}
}

func (k *heldKeys) Keep(provider, key string) error {
	if key == k.holdKey {
		k.reached <- struct{}{}
		<-k.release
	}
	return k.FileKeys.Keep(provider, key)
}

func (k *heldKeys) Forget(provider string) error {
	if k.holdForget {
		k.reached <- struct{}{}
		<-k.release
	}
	return k.FileKeys.Forget(provider)
}

// within is what a change gave, if it finished in a moment: long enough for one that is not held back.
func within(done <-chan error, wait time.Duration) (err error, finished bool) {
	select {
	case err := <-done:
		return err, true
	case <-time.After(wait):
		return nil, false
	}
}

func usableKey(t *testing.T, r *Registry, provider string) string {
	t.Helper()
	if err := r.Enable(provider, ""); err != nil {
		t.Fatal(err)
	}
	got := r.Effective()
	if problem := got.Problems[provider]; problem != "" {
		t.Fatalf("%s is in the registry and cannot be used: %s", provider, problem)
	}
	return got.Credentials[provider].Key
}

func TestTwoAdminsAddingTheSameProviderAtOnceLeaveTheOneAddedWithItsKey(t *testing.T) {
	r, dir := opened(t, Rules{})
	keys := holding(dir + "/keys")
	keys.holdKey = "sk-first"
	r.UseKeys(keys)
	first, second := theirs("acme"), theirs("acme")
	first.Key, second.Key = "sk-first", "sk-second"

	firstDone := make(chan error, 1)
	go func() { firstDone <- r.AddProvider(first) }()
	<-keys.reached
	secondDone := make(chan error, 1)
	go func() { secondDone <- r.AddProvider(second) }()
	secondErr, finished := within(secondDone, 200*time.Millisecond)
	close(keys.release)
	firstErr := <-firstDone
	if !finished {
		if secondErr, finished = within(secondDone, 5*time.Second); !finished {
			t.Fatal("the second admin's change never finished")
		}
	}
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("one of the two is added and the other refused: %v, %v", firstErr, secondErr)
	}
	want := "sk-first"
	if firstErr != nil {
		want = "sk-second"
	}
	if got := usableKey(t, r, "acme"); got != want {
		t.Errorf("the provider that was added is used with its own key, %q, and not %q", want, got)
	}
}

func TestARetiredProviderAddedAgainKeepsItsNewKey(t *testing.T) {
	r, dir := opened(t, Rules{})
	keys := holding(dir + "/keys")
	r.UseKeys(keys)
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Fatal(err)
	}
	keys.holdForget = true
	retired := make(chan error, 1)
	go func() { retired <- r.RetireProvider("acme") }()
	<-keys.reached
	keys.holdForget = false

	again := theirs("acme")
	again.Key = "sk-the-new-key"
	added := make(chan error, 1)
	go func() { added <- r.AddProvider(again) }()
	addErr, finished := within(added, 200*time.Millisecond)
	close(keys.release)
	if err := <-retired; err != nil {
		t.Fatal(err)
	}
	if !finished {
		if addErr, finished = within(added, 5*time.Second); !finished {
			t.Fatal("adding the provider again never finished")
		}
	}
	if addErr != nil {
		t.Fatal(addErr)
	}
	if got := usableKey(t, r, "acme"); got != "sk-the-new-key" {
		t.Errorf("the provider added again is used with the key it was given, not %q", got)
	}
}

func TestARetiredProvidersModelsAreRetiredWithItAndCannotBeChanged(t *testing.T) {
	r, _ := opened(t, Rules{})
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Fatal(err)
	}
	if err := r.AddModel("acme", "big-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.RetireProvider("acme"); err != nil {
		t.Fatal(err)
	}
	if model := r.Document().Models[0]; !model.Retired {
		t.Errorf("a retired provider's model is retired with it: %+v", model)
	}
	price := &spend.Price{InputPerMillion: 1, OutputPerMillion: 2}
	for name, err := range map[string]error{
		"priced": r.SetPrice("acme", "big-1", price), "turned on": r.Enable("acme", "big-1"),
	} {
		if err != ErrNoSuch {
			t.Errorf("a retired provider's model cannot be %s: %v", name, err)
		}
	}
}

func TestAModelKeptLiveUnderARetiredProviderCannotBeChanged(t *testing.T) {
	r, _ := opened(t, Rules{})
	if err := r.AddProvider(theirs("acme")); err != nil {
		t.Fatal(err)
	}
	if err := r.AddModel("acme", "big-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.change(func(d *Document) error {
		d.Providers[0].Retired = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, change := range []func() error{
		func() error { return r.SetPrice("acme", "big-1", nil) },
		func() error { return r.Enable("acme", "big-1") },
		func() error { return r.RetireModel("acme", "big-1") },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := change(); err != ErrNoSuch {
				t.Errorf("a model whose provider is retired is not changed: %v", err)
			}
		}()
	}
	wg.Wait()
}
