package cloud

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestARealAgentProposesSeatsThroughTheBillingEndpoint puts a real model in front of the endpoint. The codex program
// is the agent: the billing endpoint is the only MCP server it is given, reached over HTTP with the organization's
// token, and a person's yes is in the prompt. It uses a model and costs a little, so it runs only with GUSTO_REAL_AGENT=1.
func TestARealAgentProposesSeatsThroughTheBillingEndpoint(t *testing.T) {
	if os.Getenv("GUSTO_REAL_AGENT") != "1" {
		t.Skip("set GUSTO_REAL_AGENT=1 to run a real agent against the billing endpoint")
	}
	program, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("the codex program is not installed")
	}
	b := newBilled(t)
	_, id := b.subscribe()
	b.activate(id, "WH-1")
	token := b.token()
	dir, answer := t.TempDir(), filepath.Join(t.TempDir(), "answer.txt")
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	prompt := "I am this organization's admin. Two colleagues join on Monday and I confirm now that you should propose the seats we need. " +
		"Check how billing stands with the billing tools, then propose the number of seats that covers everyone, with a one-sentence reason. Then tell me what happens next."
	cmd := exec.CommandContext(ctx, program, "exec", "--skip-git-repo-check", "--sandbox", "read-only", "-C", dir, "-o", answer,
		"-c", `mcp_servers.billing.url="`+b.front.http.URL+`/billing/mcp"`,
		"-c", `mcp_servers.billing.bearer_token_env_var="GUSTO_BILLING_TOKEN"`,
		"-c", `mcp_servers.billing.default_tools_approval_mode="approve"`, prompt)
	cmd.Env = append(os.Environ(), "GUSTO_BILLING_TOKEN="+token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the agent failed: %v\n%s", err, out)
	}
	open, _ := b.front.store.OpenProposals(b.org)
	if len(open) != 1 || open[0].Seats != 3 || open[0].Source != "agent" || open[0].Status != ProposalPending {
		t.Fatalf("the agent's proposal: %+v\n%s", open, out)
	}
	if b.record().Seats != 1 || b.fake.count("POST /v1/billing/subscriptions/"+id+"/revise") != 0 {
		t.Errorf("the agent changed seats or reached PayPal")
	}
	said, _ := os.ReadFile(answer)
	if !strings.Contains(strings.ToLower(string(said)), "approve") {
		t.Errorf("the agent did not say a person approves next:\n%s", said)
	}
	t.Logf("the agent said:\n%s", said)
}
