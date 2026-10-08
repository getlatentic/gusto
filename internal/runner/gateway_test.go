package runner

import (
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

func gatewaySpec() job.Spec {
	return job.Spec{Setup: &job.Setup{Name: "team", Gateway: []string{"crm", "billing"}, Servers: []string{"memory"}}}
}

func TestAJobWithServersBehindTheGatewayIsGivenOneServerAndNotTheirDefinitions(t *testing.T) {
	t.Setenv("HAAS_MODEL_URL", "http://127.0.0.1:8041/model/v1")
	t.Setenv("HAAS_MODEL_TOKEN", "job-token")
	direct := []job.Server{{Name: "memory", Command: "mcp-server-memory"}}
	got, err := withGateway(direct, gatewaySpec())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "memory" || got[1].Name != GatewayName {
		t.Fatalf("servers = %+v", got)
	}
	if got[1].URL != "http://127.0.0.1:8041/model/mcp" || got[1].Headers["Authorization"] != "Bearer job-token" {
		t.Errorf("the gateway server = %+v", got[1])
	}
	if strings.Contains(got[1].URL, "crm") || strings.Contains(got[1].Name, "crm") {
		t.Error("a server behind the gateway is named to the harness")
	}
}

func TestAJobWithNothingBehindTheGatewayIsGivenNoGateway(t *testing.T) {
	t.Setenv("HAAS_MODEL_TOKEN", "job-token")
	for _, spec := range []job.Spec{{}, {Setup: &job.Setup{Name: "x", Servers: []string{"memory"}}}} {
		got, err := withGateway([]job.Server{{Name: "memory"}}, spec)
		if err != nil || len(got) != 1 {
			t.Errorf("%+v %v", got, err)
		}
	}
}

func TestAJobWithServersBehindTheGatewayAndNoProxyIsRefusedByName(t *testing.T) {
	t.Setenv("HAAS_MODEL_TOKEN", "")
	if _, err := withGateway(nil, gatewaySpec()); err == nil || !strings.Contains(err.Error(), "no proxy") || !strings.Contains(err.Error(), "team") {
		t.Errorf("err = %v", err)
	}
}

func TestAHarnessWithNoMCPIsRefusedAGatewayJob(t *testing.T) {
	spec := job.Spec{Setup: &job.Setup{Name: "team", Gateway: []string{"crm"}}}
	if err := fits(spec, harness.Adapter{Name: "plain"}); err == nil || !strings.Contains(err.Error(), "takes no MCP servers") {
		t.Errorf("err = %v", err)
	}
}
