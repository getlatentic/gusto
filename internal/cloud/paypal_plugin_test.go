package cloud

import (
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/integration"
)

// The plugin that gives an agent the billing tools is read by the same reader an administrator's import uses.
func TestTheBillingPluginIsAnAgentPluginWithTheServerAndTheSkill(t *testing.T) {
	plugin, err := integration.Read(filepath.Join("..", "..", "deploy", "plugins", "gusto-billing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugin.Problems) != 0 || len(plugin.Servers) != 1 || plugin.Servers[0].Name != "billing" || plugin.Servers[0].Transport != integration.StreamableHTTP {
		t.Errorf("%+v", plugin)
	}
	if len(plugin.Skills) != 1 || plugin.Skills[0].Name != "seat-changes" {
		t.Errorf("%+v", plugin.Skills)
	}
	found := false
	for _, tool := range billingTools {
		found = found || tool["name"] == "billing_propose_seats"
	}
	if !found {
		t.Errorf("the skill names a tool the endpoint does not have")
	}
}
