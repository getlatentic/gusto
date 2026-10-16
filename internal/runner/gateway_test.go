package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
)

func gatewaySpec() job.Spec {
	return job.Spec{Setup: &job.Setup{Name: "team", Gateway: []string{"crm", "billing"}, Servers: []string{"memory"}}}
}

func TestAJobWithServersBehindTheGatewayIsGivenOneServerAndNotTheirDefinitions(t *testing.T) {
	t.Setenv("GUSTO_MODEL_URL", "http://127.0.0.1:8041/model/v1")
	t.Setenv("GUSTO_MODEL_TOKEN", "job-token")
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
	t.Setenv("GUSTO_MODEL_TOKEN", "job-token")
	for _, spec := range []job.Spec{{}, {Setup: &job.Setup{Name: "x", Servers: []string{"memory"}}}} {
		got, err := withGateway([]job.Server{{Name: "memory"}}, spec)
		if err != nil || len(got) != 1 {
			t.Errorf("%+v %v", got, err)
		}
	}
}

func TestAJobWithServersBehindTheGatewayAndNoProxyIsRefusedByName(t *testing.T) {
	t.Setenv("GUSTO_MODEL_TOKEN", "")
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

func inABox(t *testing.T, name string) (harness.Adapter, harness.Env) {
	t.Helper()
	adapter, err := harness.Load("", name)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, harness.Env{Boxed: true, ModelURL: "http://host.openshell.internal:8041/model/v1", ModelToken: "box-token"}
}

func TestAHarnessInABoxIsGivenTheGatewayAtTheProxyItReachesItsModelAt(t *testing.T) {
	for _, name := range []string{"claude", "codex", "opencode"} {
		adapter, env := inABox(t, name)
		command, servers, err := boxCommand(context.Background(), adapter, gatewaySpec(), "/sandbox/work", env)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(servers) != 1 || servers[0].URL != "http://host.openshell.internal:8041/model/mcp" || servers[0].Headers["Authorization"] != "Bearer box-token" {
			t.Errorf("%s: servers = %+v", name, servers)
		}
		line := strings.Join(append(command.Args, command.Env...), " ")
		if !strings.Contains(line, "host.openshell.internal:8041/model/mcp") || !strings.Contains(line, "box-token") {
			t.Errorf("%s: the command does not carry the gateway: %s", name, line)
		}
		if strings.Contains(line, "crm") || strings.Contains(line, "billing") {
			t.Errorf("%s: a server behind the gateway is named to the harness", name)
		}
	}
}

func TestAnACPHarnessInABoxIsGivenTheGatewayOnItsSession(t *testing.T) {
	adapter, env := inABox(t, "hermes")
	command, servers, err := boxCommand(context.Background(), adapter, gatewaySpec(), "/sandbox/work", env)
	if err != nil || len(servers) != 1 || servers[0].Name != GatewayName {
		t.Fatalf("%+v %v", servers, err)
	}
	if strings.Contains(strings.Join(command.Args, " "), "model/mcp") {
		t.Error("an ACP harness is given its servers on the session and not on its command line")
	}
}

func TestAHarnessThatReadsItsServersFromAFileIsRefusedInABoxByName(t *testing.T) {
	adapter, env := inABox(t, "pi")
	if _, _, err := boxCommand(context.Background(), adapter, gatewaySpec(), "/sandbox/work", env); err == nil || !strings.Contains(err.Error(), "pi reads its servers from a file") {
		t.Errorf("err = %v", err)
	}
}

func TestAJobWithNothingBehindTheGatewayIsGivenNothingInABoxAndOneWithNoTokenIsRefused(t *testing.T) {
	adapter, env := inABox(t, "claude")
	command, servers, err := boxCommand(context.Background(), adapter, job.Spec{Setup: &job.Setup{Name: "x"}}, "/w", env)
	if err != nil || len(servers) != 0 || strings.Contains(strings.Join(command.Args, " "), "mcp-config") {
		t.Errorf("%+v %v %v", servers, err, command.Args)
	}
	env.ModelToken = ""
	if _, _, err := boxCommand(context.Background(), adapter, gatewaySpec(), "/w", env); err == nil || !strings.Contains(err.Error(), "no proxy") {
		t.Errorf("err = %v", err)
	}
}
