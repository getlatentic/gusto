package cloud

import (
	"errors"
	"strings"
	"testing"
)

func TestAnAddressIsKeptInOneSpelling(t *testing.T) {
	got, err := NormalEmail("  Ada@Example.COM ")
	if err != nil || got != "ada@example.com" {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"", "ada", "ada@", "@example.com", "ada@example", "Ada <ada@example.com>", "a b@example.com",
		"ada@example.com, bob@example.com", "ada@.example.com", "ada@example..com", strings.Repeat("a", 65) + "@example.com",
		"ada@example.com\r\nBcc: x@y.com"} {
		if _, err := NormalEmail(bad); !errors.Is(err, ErrBadEmail) {
			t.Errorf("%q should not be an address: %v", bad, err)
		}
	}
}

func TestACompanyAddressIsAskedForAndAFreeOneIsTurnedAwayUnlessAllowed(t *testing.T) {
	if err := workEmail("ada@gmail.com", false, nil); !errors.Is(err, ErrFreeMail) {
		t.Errorf("a free mail address should be turned away: %v", err)
	}
	if err := workEmail("ada@gmail.com", true, nil); err != nil {
		t.Errorf("a demonstration allows it: %v", err)
	}
	if err := workEmail("ada@mailinator.com", true, nil); !errors.Is(err, ErrDisposable) {
		t.Errorf("an address that expires is never allowed: %v", err)
	}
	if err := workEmail("ada@acme.test", false, setOf("acme.test")); !errors.Is(err, ErrDisposable) {
		t.Errorf("the operator's own list applies: %v", err)
	}
	if err := workEmail("ada@acme.com", false, nil); err != nil {
		t.Error(err)
	}
}

func TestAnOrganisationsNameIsTextAndItsAddressIsMadeFromIt(t *testing.T) {
	name, err := OrgName("  Acme   Research  ")
	if err != nil || name != "Acme Research" {
		t.Fatalf("%q %v", name, err)
	}
	for _, bad := range []string{"", "A", strings.Repeat("x", 81), "Acme <script>", "Acme\x00", "Acme‮evil"} {
		if _, err := OrgName(bad); err == nil {
			t.Errorf("%q should not be a name", bad)
		}
	}
	for name, want := range map[string]string{
		"Acme Research": "acme-research", "  --Acme!! & Sons--": "acme-sons", "Åcme Ltd": "cme-ltd",
		"A": "a", strings.Repeat("ab ", 20): "ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab",
	} {
		if got := SlugFor(name); got != want {
			t.Errorf("SlugFor(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestOnlyAnAddressAHostNameCanTakeIsAllowed(t *testing.T) {
	for _, good := range []string{"acme", "acme-research", "a1b", strings.Repeat("a", 32)} {
		if err := ValidSlug(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
	for _, bad := range []string{"", "ab", "-acme", "acme-", "Acme", "ac me", "acme.com", "ac--me", "xn--acme", strings.Repeat("a", 33), "ac_me"} {
		if err := ValidSlug(bad); !errors.Is(err, ErrBadSlug) {
			t.Errorf("%q should be refused as an address: %v", bad, err)
		}
	}
	for _, kept := range []string{"www", "admin", "api", "haas", "localhost"} {
		if err := ValidSlug(kept); !errors.Is(err, ErrReservedSlug) {
			t.Errorf("%q should be kept: %v", kept, err)
		}
	}
}
