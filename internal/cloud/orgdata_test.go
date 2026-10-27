package cloud

import (
	"strings"
	"testing"
)

// Everything an organization's people make is kept in the organization's own directory, since servers on one host start in one
// directory and a setting left to its default would be shared by all of them.
func TestWhatAnOrganizationsPeopleMakeIsKeptInTheOrganizationsOwnDirectory(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		own := map[string]map[string]string{}
		for _, slug := range []string{"acme", "globex"} {
			env := environmentFor(t, nil, slug)
			own[slug] = map[string]string{}
			for _, name := range []string{"GUSTO_JOBS", "GUSTO_SESSIONS", "GUSTO_SETUPS", "GUSTO_CREW", "GUSTO_PRESETS", "GUSTO_INTEGRATIONS", "GUSTO_COMPARISONS"} {
				value, count := settingOf(env, name)
				if count != 1 || !strings.Contains(value, "/"+slug+"/") {
					t.Errorf("%s for %s is %q (set %d times), which is not the organization's own", name, slug, value, count)
				}
				own[slug][name] = value
			}
		}
		for name, value := range own["acme"] {
			if value == own["globex"][name] {
				t.Errorf("two organizations share %s: %q", name, value)
			}
		}
	})
}
