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
	if person.Email != "ada@acme.com" || person.Role != Admin || person.Org != "acme-research" || !person.Trial() {
		t.Errorf("%+v", person)
	}
	if org.Slug != "acme-research" || org.Name != "Acme Research" || org.Admin != "ada@acme.com" {
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
		"the same address":     {Signup{Email: "ADA@acme.com", OrgName: "Other"}, ErrEmailTaken},
		"the same slug":        {Signup{Email: "bob@example.org", OrgName: "Acme"}, ErrSlugTaken},
		"a reserved slug":      {Signup{Email: "bob@example.org", OrgName: "Bob", Slug: "admin"}, ErrReservedSlug},
		"free mail":            {Signup{Email: "bob@gmail.com", OrgName: "Bob Ltd"}, ErrFreeMail},
		"a name with no slug":  {Signup{Email: "bob@example.org", OrgName: "!!"}, ErrBadSlug},
		"a name of one letter": {Signup{Email: "bob@example.org", OrgName: "B"}, ErrBadOrgName},
	} {
		if _, _, err := s.SignUp(in.in); !errors.Is(err, in.want) {
			t.Errorf("%s: %v, want %v", name, err, in.want)
		}
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

func TestOneCompanyMayMakeOnlyAFewOrganisations(t *testing.T) {
	s := open(t, Settings{PerDomain: 2})
	for i := range 2 {
		if _, _, err := s.SignUp(Signup{Email: fmt.Sprintf("u%d@acme.com", i), OrgName: fmt.Sprintf("Team %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.SignUp(Signup{Email: "u3@acme.com", OrgName: "Team 3"}); !errors.Is(err, ErrDomainFull) {
		t.Errorf("%v", err)
	}
	if _, _, err := s.SignUp(Signup{Email: "u3@other.com", OrgName: "Team 3"}); err != nil {
		t.Errorf("another company is not held to it: %v", err)
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
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("p%d@company%d.com", i, i), OrgName: "Contested"})
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
