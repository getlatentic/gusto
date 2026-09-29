package cloud

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func open(t *testing.T, set Settings) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cloud"), set)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestASignUpMakesAPersonAnOrganisationAndItsDirectoryOnTheTrial(t *testing.T) {
	s := open(t, Settings{})
	person, org, err := s.SignUp(Signup{Email: "Ada@Acme.com", OrgName: "Acme Research"})
	if err != nil {
		t.Fatal(err)
	}
	if person.Email != "ada@acme.com" || person.Role != Admin || person.Org != "acme-com" || !person.Trial() {
		t.Errorf("%+v", person)
	}
	if org.Slug != "acme-com" || org.Name != "Acme Research" || org.Admin != "ada@acme.com" || org.Domain != "acme.com" {
		t.Errorf("%+v", org)
	}
	got, found, err := s.Person("ADA@acme.com")
	if err != nil || !found || got.Org != org.Slug {
		t.Errorf("the person should be found by any spelling: %+v %v %v", got, found, err)
	}
	quota, err := s.Quota(org.Slug)
	if err != nil || quota != DefaultLimits().Trial {
		t.Errorf("a sign-up starts on the trial: %+v %v", quota, err)
	}
	for _, dir := range []string{s.OrgDir(org.Slug), s.DataDir(org.Slug)} {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("%s should be private to the server: %v %v", dir, info, err)
		}
	}
}

func TestVerifyingLiftsTheQuotaOfTheOrganisationTheirAdminMade(t *testing.T) {
	s := open(t, Settings{})
	_, org, _ := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"})
	person, err := s.Verify("ada@acme.com")
	if err != nil || person.Trial() {
		t.Fatalf("%+v %v", person, err)
	}
	if quota, _ := s.Quota(org.Slug); quota != DefaultLimits().Verified {
		t.Errorf("a verified organisation has the larger quota: %+v", quota)
	}
	again, err := s.Verify("ada@acme.com")
	if err != nil || !again.Verified.Equal(person.Verified) {
		t.Errorf("verifying twice changes nothing: %+v %v", again, err)
	}
	if _, err := s.Verify("nobody@acme.com"); err == nil {
		t.Error("an address with no account cannot be verified")
	}
}

func TestASignUpThatIsRefusedLeavesNothingBehind(t *testing.T) {
	s := open(t, Settings{})
	if _, _, err := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"}); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]struct {
		in   Signup
		want error
	}{
		"the same address":        {Signup{Email: "ADA@acme.com", OrgName: "Other"}, ErrEmailTaken},
		"a chosen address in use": {Signup{Email: "bob@example.org", OrgName: "Acme", Slug: "acme-com"}, ErrSlugTaken},
		"a reserved slug":         {Signup{Email: "bob@example.org", OrgName: "Bob", Slug: "admin"}, ErrReservedSlug},
		"free mail":               {Signup{Email: "bob@gmail.com", OrgName: "Bob Ltd"}, ErrFreeMail},
		"a name of one letter":    {Signup{Email: "bob@example.org", OrgName: "B"}, ErrBadOrgName},
	} {
		if _, _, err := s.SignUp(in.in); !errors.Is(err, in.want) {
			t.Errorf("%s: %v, want %v", name, err, in.want)
		}
	}
	// A free mail address has only its name to make an address from.
	free := open(t, Settings{AllowFreeMail: true})
	if _, _, err := free.SignUp(Signup{Email: "bob@gmail.com", OrgName: "!!"}); !errors.Is(err, ErrBadSlug) {
		t.Errorf("a name with no letters gives no address: %v", err)
	}
	// A slug taken by a refused sign-up is not held by it.
	if _, _, err := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Elsewhere", Slug: "elsewhere"}); !errors.Is(err, ErrEmailTaken) {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.OrgDir("elsewhere")); err == nil {
		t.Error("a refused sign-up left its organisation's directory")
	}
	if _, _, err := s.SignUp(Signup{Email: "eve@example.net", OrgName: "Elsewhere", Slug: "elsewhere"}); err != nil {
		t.Errorf("the slug should be free again: %v", err)
	}
}

// Two people racing for one address or one slug: exactly one wins, and the loser
// leaves no organisation behind.
func TestTwoSignUpsForOneNameHaveOneWinner(t *testing.T) {
	s := open(t, Settings{PerDomain: 100})
	var wg sync.WaitGroup
	results := make(chan error, 40)
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("p%d@company%d.com", i, i), OrgName: "Contested", Slug: "contested"})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	won := 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrSlugTaken):
			t.Errorf("a loser should be told the address is taken: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d sign-ups won the address", won)
	}
	if people, _ := os.ReadDir(filepath.Join(s.Root(), "people")); len(people) != 1 {
		t.Errorf("%d people were left, and one won", len(people))
	}
}

func TestADirectoryWithNoRecordIsSetAsideWhenTheStoreIsOpened(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cloud")
	s, err := Open(root, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	half := s.OrgDir("half-made")
	if err := os.MkdirAll(filepath.Join(half, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root, Settings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(half); err == nil {
		t.Error("a sign-up that stopped part way still holds its address")
	}
}

func TestALinkWorksOnceForItsPurposeAndBeforeItRunsOut(t *testing.T) {
	s := open(t, Settings{})
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	secret, err := s.MakeLink("ada@acme.com", Verify, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UseLink(secret, SignIn); !errors.Is(err, ErrLink) {
		t.Errorf("a link to verify an address does not sign anybody in: %v", err)
	}
	if _, err := s.UseLink(secret, Verify); !errors.Is(err, ErrLink) {
		t.Errorf("a link tried for the wrong purpose is spent by it: %v", err)
	}

	secret, _ = s.MakeLink("ada@acme.com", Verify, time.Hour)
	if email, err := s.UseLink(secret, Verify); err != nil || email != "ada@acme.com" {
		t.Fatalf("%q %v", email, err)
	}
	if _, err := s.UseLink(secret, Verify); !errors.Is(err, ErrLink) {
		t.Errorf("a link works once: %v", err)
	}

	secret, _ = s.MakeLink("ada@acme.com", SignIn, time.Minute)
	now = now.Add(2 * time.Minute)
	if _, err := s.UseLink(secret, SignIn); !errors.Is(err, ErrLink) {
		t.Errorf("a link that ran out does not work: %v", err)
	}
	for _, bad := range []string{"", "nonsense", string(make([]byte, 200))} {
		if _, err := s.UseLink(bad, Verify); !errors.Is(err, ErrLink) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestTwoRequestsWithOneLinkHaveOneWinner(t *testing.T) {
	s := open(t, Settings{})
	secret, _ := s.MakeLink("ada@acme.com", SignIn, time.Hour)
	var wg sync.WaitGroup
	var mu sync.Mutex
	in := 0
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.UseLink(secret, SignIn); err == nil {
				mu.Lock()
				in++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if in != 1 {
		t.Fatalf("%d requests used one link", in)
	}
}

func TestOnlyTheHashOfALinkIsKeptAndRunOutLinksAreSwept(t *testing.T) {
	s := open(t, Settings{})
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	kept, _ := s.MakeLink("a@acme.com", Verify, time.Hour)
	s.MakeLink("b@acme.com", Verify, time.Minute)
	files, _ := os.ReadDir(filepath.Join(s.Root(), "tokens"))
	for _, file := range files {
		if text, _ := os.ReadFile(filepath.Join(s.Root(), "tokens", file.Name())); strings.Contains(string(text), kept) || strings.Contains(file.Name(), kept) {
			t.Errorf("%s holds the secret itself", file.Name())
		}
	}
	now = now.Add(10 * time.Minute)
	if removed, err := s.Sweep(); err != nil || removed != 1 {
		t.Fatalf("removed %d, %v", removed, err)
	}
	if _, err := s.UseLink(kept, Verify); err != nil {
		t.Errorf("a link that has not run out survives the sweep: %v", err)
	}
}

func TestAFileThatMustBeNewIsMadeByOneCallerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.json")
	if err := createFile(path, map[string]string{"a": "1"}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := createFile(path, map[string]string{"a": "2"}, 0o600); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("a second create should find the file there: %v", err)
	}
	var got map[string]string
	if err := readFile(path, &got); err != nil || got["a"] != "1" {
		t.Errorf("the first file should be untouched: %v %v", got, err)
	}
}

func TestAnOrganisationIsNamedFromTheCompanysDomainWhenTheAdminGivesNoName(t *testing.T) {
	s := open(t, Settings{})
	person, org, err := s.SignUp(Signup{Email: "ada@acme-research.com"})
	if err != nil {
		t.Fatal(err)
	}
	if org.Name != "Acme Research" || org.Slug != "acme-research-com" || org.Domain != "acme-research.com" || person.Org != "acme-research-com" {
		t.Errorf("%+v %+v", person, org)
	}
	// A name given is kept, and the address still follows from it.
	_, other, err := s.SignUp(Signup{Email: "bob@globex.com", OrgName: "Globex Secret Works"})
	if err != nil || other.Slug != "globex-com" || other.Name != "Globex Secret Works" || other.Domain != "globex.com" {
		t.Errorf("%+v %v", other, err)
	}
}

func TestAFreeMailAddressHasNoCompanyToNameItAndClaimsNoDomain(t *testing.T) {
	s := open(t, Settings{AllowFreeMail: true})
	if _, _, err := s.SignUp(Signup{Email: "ada@gmail.com"}); !errors.Is(err, ErrNeedName) {
		t.Fatalf("a name is asked for: %v", err)
	}
	_, org, err := s.SignUp(Signup{Email: "ada@gmail.com", OrgName: "Ada's Lab"})
	if err != nil || org.Domain != "" {
		t.Fatalf("%+v %v", org, err)
	}
	if _, err := s.Verify("ada@gmail.com"); err != nil {
		t.Fatal(err)
	}
	// Nor is a second person at the same provider held off by the first.
	if _, _, err := s.SignUp(Signup{Email: "bob@gmail.com", OrgName: "Bob's Lab"}); err != nil {
		t.Errorf("%v", err)
	}
}

// An address that comes from a company's domain is always one an organisation can
// have, and is that company's alone: the whole registrable domain, so two companies
// whose names are alike each have one, and neither is put ahead of the other.
func TestAnAddressFromADomainIsTheWholeDomainAndIsAlwaysUsable(t *testing.T) {
	s := open(t, Settings{})
	for email, want := range map[string]string{
		"ada@test.com":              "test-com", // a kept word is only kept on its own
		"ada@ab.com":                "ab-com",
		"ada@admin.org":             "admin-org",
		"ada@mail.acme.co.uk":       "acme-co-uk",
		"ada@acme-research.com":     "acme-research-com",
		"bob@acme-research.co":      "acme-research-co",
		"cy@acme-research.io":       "acme-research-io",
		"di@team.acme-research.net": "acme-research-net",
	} {
		_, org, err := s.SignUp(Signup{Email: email})
		if err != nil || org.Slug != want || org.Name == "" {
			t.Errorf("%s: %+v %v, want %q", email, org, err, want)
		}
	}
}

func TestAnAddressFromAVeryLongDomainStaysWithinWhatAHostNameTakesAndDifferentFromAnotherLongOne(t *testing.T) {
	long1, long2 := AddressFor("international-consolidated-holdings-limited.com"), AddressFor("international-consolidated-holdings-limited.org")
	for _, slug := range []string{long1, long2} {
		if len(slug) > 32 || ValidSlug(slug) != nil {
			t.Errorf("%q does not make an address", slug)
		}
	}
	if long1 == long2 || long1[:20] != long2[:20] {
		t.Errorf("two long domains that begin alike are two addresses that begin alike: %q %q", long1, long2)
	}
	if AddressFor("acme.com") != "acme-com" {
		t.Error("a short one is the whole domain")
	}
}

// A company is more than one organisation when it has subsidiaries or teams that
// keep apart. The first has the address its domain gives, and each after it is
// named by whoever makes it and has an address from that name.
func TestACompanyCanHaveSeveralOrganisationsAndEachAfterTheFirstIsNamedByItsCreator(t *testing.T) {
	s := open(t, Settings{PerDomain: 10})
	_, first, err := s.SignUp(Signup{Email: "ada@acme.com"})
	if err != nil || first.Slug != "acme-com" || first.Name != "Acme" || first.Domain != "acme.com" {
		t.Fatalf("%+v %v", first, err)
	}
	if _, _, err := s.SignUp(Signup{Email: "bob@acme.com"}); !errors.Is(err, ErrSameCompany) {
		t.Fatalf("a second with no name is asked for one: %v", err)
	}
	_, bank, err := s.SignUp(Signup{Email: "bob@acme.com", OrgName: "Acme Bank"})
	if err != nil || bank.Slug != "acme-bank" || bank.Name != "Acme Bank" || bank.Domain != "acme.com" {
		t.Fatalf("%+v %v", bank, err)
	}
	_, insurance, err := s.SignUp(Signup{Email: "cy@mail.acme.com", OrgName: "Acme Insurance"})
	if err != nil || insurance.Slug != "acme-insurance" || insurance.Domain != "acme.com" {
		t.Fatalf("an address at a subdomain is the same company: %+v %v", insurance, err)
	}
	if _, _, err := s.SignUp(Signup{Email: "di@acme.com", OrgName: "Acme Bank"}); !errors.Is(err, ErrSlugTaken) {
		t.Errorf("a name that gives a taken address is refused as taken: %v", err)
	}
	if _, org, err := s.SignUp(Signup{Email: "ed@acme.com", OrgName: "Acme Labs", Slug: "labs"}); err != nil || org.Slug != "labs" {
		t.Errorf("an address chosen on purpose is used: %+v %v", org, err)
	}
}

// Nobody is kept from making an organisation because their company has one. What is
// limited is organisations nobody has confirmed an address for, which is what
// making many to hold names or to get many trials looks like.
func TestOnlyOrganisationsNobodyHasConfirmedAreLimitedForOneCompany(t *testing.T) {
	s := open(t, Settings{PerDomain: 2})
	s.SignUp(Signup{Email: "a@acme.com"})
	s.SignUp(Signup{Email: "b@acme.com", OrgName: "Beta"})
	if _, _, err := s.SignUp(Signup{Email: "c@acme.com", OrgName: "Gamma"}); !errors.Is(err, ErrDomainFull) {
		t.Fatalf("a third that nobody has confirmed: %v", err)
	}
	if _, err := s.Verify("a@acme.com"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SignUp(Signup{Email: "c@acme.com", OrgName: "Gamma"}); err != nil {
		t.Fatalf("one confirmed leaves room: %v", err)
	}
	s.Verify("b@acme.com")
	s.Verify("c@acme.com")
	for _, who := range []string{"d", "e", "f", "g"} {
		if _, _, err := s.SignUp(Signup{Email: who + "@acme.com", OrgName: "Org " + who}); err != nil {
			t.Fatalf("a company with all of its organisations confirmed is not held to a number: %v", err)
		}
		s.Verify(who + "@acme.com")
	}
	if _, _, err := s.SignUp(Signup{Email: "z@acme.co", OrgName: "Elsewhere"}); err != nil {
		t.Errorf("another company is not held to it: %v", err)
	}
}

// Two people at one company who both make the first organisation at once: one gets
// the domain's address and the other is asked for a name of its own.
func TestTwoSignUpsForOneCompanysFirstOrganisationAtOnceHaveOneWinner(t *testing.T) {
	s := open(t, Settings{PerDomain: 100})
	var wg sync.WaitGroup
	results := make(chan error, 30)
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("p%d@acme.com", i)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	won := 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrSameCompany):
			t.Errorf("a loser is asked for a name: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d organisations have the domain's address", won)
	}
}
