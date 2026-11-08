package entry

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var key = []byte("0123456789abcdef0123456789abcdef")

func TestATokenIsGoodForItsOrganizationAndItsPersonForAMinute(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	token, err := Mint(key, "acme-com", "ada@acme.com", "admin", now)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := Verify(key, "acme-com", token, now.Add(30*time.Second))
	if err != nil || claims.Email != "ada@acme.com" || claims.Role != "admin" || claims.Org != "acme-com" {
		t.Fatalf("%+v %v", claims, err)
	}
	if _, err := Verify(key, "acme-com", token, now.Add(Life)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token runs out: %v", err)
	}
	if _, err := Verify(key, "globex-com", token, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token for one organization is not good in another: %v", err)
	}
	if _, err := Verify([]byte("another key another key another!"), "acme-com", token, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token made with another key is not one: %v", err)
	}
}

func TestATokenThatWasChangedOrIsNotOneIsInvalid(t *testing.T) {
	now := time.Now()
	token, _ := Mint(key, "acme-com", "ada@acme.com", "runner", now)
	payload, signature, _ := strings.Cut(token, ".")
	other, _ := Mint(key, "acme-com", "bob@acme.com", "admin", now)
	otherPayload, _, _ := strings.Cut(other, ".")
	for name, bad := range map[string]string{
		"a payload with another's signature": otherPayload + "." + signature,
		"no signature":                       payload,
		"a truncated signature":              token[:len(token)-3],
		"empty":                              "",
		"only a dot":                         ".",
		"not base64":                         "!!!." + signature,
	} {
		if _, err := Verify(key, "acme-com", bad, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestATokenIsSpentOnceEvenWhenTriedTogether(t *testing.T) {
	now := time.Now()
	token, _ := Mint(key, "acme-com", "ada@acme.com", "admin", now)
	claims, _ := Verify(key, "acme-com", token, now)
	var seen Seen
	var wg sync.WaitGroup
	var mu sync.Mutex
	in := 0
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if seen.Use(claims, now) {
				mu.Lock()
				in++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if in != 1 {
		t.Fatalf("%d entries with one token", in)
	}
	// What was spent is forgotten once it would have run out, so the record does not grow.
	later := now.Add(2 * Life)
	other, _ := Mint(key, "acme-com", "bob@acme.com", "admin", later)
	otherClaims, _ := Verify(key, "acme-com", other, later)
	seen.Use(otherClaims, later)
	seen.mu.Lock()
	held := len(seen.used)
	seen.mu.Unlock()
	if held != 1 {
		t.Errorf("%d nonces are held", held)
	}
}

func TestTwoTokensForOnePersonAreTwo(t *testing.T) {
	now := time.Now()
	a, _ := Mint(key, "acme-com", "ada@acme.com", "admin", now)
	b, _ := Mint(key, "acme-com", "ada@acme.com", "admin", now)
	if a == b {
		t.Error("tokens repeat")
	}
}

func TestATokenToSignOutWithDoesNotLetAnybodyIn(t *testing.T) {
	now := time.Now()
	exit, err := MintExit(key, "acme-com", "ada@acme.com", now)
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := VerifyExit(key, "acme-com", exit, now); err != nil || claims.Email != "ada@acme.com" || claims.Role != "" {
		t.Fatalf("%+v %v", claims, err)
	}
	if _, err := Verify(key, "acme-com", exit, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token to sign out with signs somebody in: %v", err)
	}
	enter, _ := Mint(key, "acme-com", "ada@acme.com", "admin", now)
	if _, err := VerifyExit(key, "acme-com", enter, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token to enter with signs somebody out: %v", err)
	}
	if _, err := VerifyExit(key, "acme-com", exit, now.Add(Life)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token to sign out with runs out: %v", err)
	}
	if _, err := VerifyExit(key, "globex-com", exit, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a token to sign out of one organization is taken by another: %v", err)
	}
}
