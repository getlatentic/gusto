package proxy

import "testing"

func TestOnlyWhatTheAllowlistNamesIsPermitted(t *testing.T) {
	list := ParseAllowlist(" Registry.npmjs.org, proxy.golang.org:8443, *.githubusercontent.com ,")
	for address, want := range map[string]bool{
		"registry.npmjs.org:443":               true,
		"registry.npmjs.org:80":                true,
		"REGISTRY.NPMJS.ORG:443":               true,
		"registry.npmjs.org:5432":              false,
		"proxy.golang.org:8443":                true,
		"proxy.golang.org:443":                 false,
		"raw.githubusercontent.com:443":        true,
		"githubusercontent.com:443":            false,
		"evilgithubusercontent.com:443":        false,
		"registry.npmjs.org.attacker.test:443": false,
		"pastebin.com:443":                     false,
		"registry.npmjs.org":                   false,
	} {
		if got := list.Permits(address); got != want {
			t.Errorf("%s: permitted %t, want %t", address, got, want)
		}
	}
}

func TestAnEmptyAllowlistPermitsNothing(t *testing.T) {
	if ParseAllowlist("").Permits("example.com:443") {
		t.Fatal("an empty allowlist let a host through")
	}
}
