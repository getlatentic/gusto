package web

import "testing"

// A provider says who somebody is. Whether they may spend this server's
// credential is a different question, and the allowlist is where it is answered.
func TestAnAddressNobodyNamedIsNotAdmitted(t *testing.T) {
	only := Access{Allowed: []string{"tosinamuda@gmail.com"}}
	if !only.admits("tosinamuda@gmail.com") {
		t.Fatal("the named address was refused")
	}
	if only.admits("TosinAmuda@Gmail.com") == false {
		t.Fatal("an address differing only in case was refused")
	}
	for _, other := range []string{"somebody@gmail.com", "tosinamuda@gmail.com.example.net", "", "tosinamuda@example.com"} {
		if only.admits(other) {
			t.Errorf("%q was admitted", other)
		}
	}
}

func TestAWholeDomainCanBeNamed(t *testing.T) {
	team := Access{Allowed: []string{"@getlatentic.com"}}
	if !team.admits("anybody@getlatentic.com") {
		t.Fatal("somebody at the named domain was refused")
	}
	for _, other := range []string{"anybody@notgetlatentic.com", "anybody@getlatentic.com.example.net", "getlatentic.com"} {
		if team.admits(other) {
			t.Errorf("%q was admitted", other)
		}
	}
}

// Without a provider the shared password is the only way in, which is what a
// deployment that names none has.
func TestWithNoProviderNamedNothingPretendsToSignPeopleIn(t *testing.T) {
	var none *Identity
	if none.Configured() {
		t.Fatal("a nil identity reported itself configured")
	}
	if (&Identity{}).Configured() {
		t.Fatal("an identity with no provider reported itself configured")
	}
}
