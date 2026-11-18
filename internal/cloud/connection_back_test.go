package cloud

import (
	"net/http"
	"testing"
)

// A person back from signing in at an app is handed on to the organization the sign-in's state names, with the app's answer
// as it came, wherever organizations are reached. A state that names no organization is not handed anywhere.
func TestASignInAtAnAppIsHandedOnToTheOrganizationItsStateNames(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		for _, at := range []struct {
			paths bool
			want  string
		}{
			{true, "http://cloud.test/workspace/acme-com/connections/callback?code=c0de&state=acme-com.r4nd0m"},
			{false, "http://acme-com.cloud.test/connections/callback?code=c0de&state=acme-com.r4nd0m"},
		} {
			f := newFront(t, Settings{}, func(c *Config) {
				c.Controller = controlled(t, c.Store)
				c.OrgPaths = at.paths
			})
			status, _, header := f.browser().get2("/connections/callback?code=c0de&state=acme-com.r4nd0m")
			if status != http.StatusSeeOther || header.Get("Location") != at.want {
				t.Errorf("at paths %v: %d to %q", at.paths, status, header.Get("Location"))
			}
			for _, state := range []string{"r4nd0m", "Not_A_Slug.r4nd0m", ""} {
				if status, _, _ := f.browser().get2("/connections/callback?code=c0de&state=" + state); status != http.StatusNotFound {
					t.Errorf("the state %q was handed on: %d", state, status)
				}
			}
		}
	})
}
