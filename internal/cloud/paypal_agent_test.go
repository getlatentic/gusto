package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

type toolAnswer struct {
	Result struct {
		Content []struct{ Text string }
		IsError bool
		Tools   []struct{ Name string }
	}
	Error *rpcError
}

// rpc calls the agent endpoint with a bearer token and returns the HTTP status and the answer.
func (b *billed) rpc(token, body string) (int, toolAnswer) {
	b.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, b.front.http.URL+"/billing/mcp", bytes.NewReader([]byte(body)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	var answer toolAnswer
	raw, _ := io.ReadAll(resp.Body)
	json.Unmarshal(raw, &answer)
	return resp.StatusCode, answer
}

func call(tool, arguments string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + arguments + `}}`
}

var agentToken = regexp.MustCompile(`gbt_[A-Za-z0-9_-]+`)

// token is what the admin is shown when they ask for one.
func (b *billed) token() string {
	b.t.Helper()
	code, page, _ := b.admin.post("/org/paypal/agent-token", nil)
	found := agentToken.FindString(page)
	if code != 200 || found == "" {
		b.t.Fatalf("the admin was given no token: %d", code)
	}
	return found
}

func TestAnAgentNeedsTheOrganizationsTokenAndAnOldTokenStopsWhenANewOneIsMade(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		if code, _ := b.rpc("", list); code != 401 {
			t.Errorf("no token answered %d", code)
		}
		if code, _ := b.rpc("gbt_wrong", list); code != 401 {
			t.Errorf("a wrong token answered %d", code)
		}
		first := b.token()
		if code, answer := b.rpc(first, list); code != 200 || len(answer.Result.Tools) != 2 {
			t.Fatalf("%d %+v", code, answer)
		}
		second := b.token()
		if code, _ := b.rpc(first, list); code != 401 {
			t.Errorf("the replaced token still worked: %d", code)
		}
		if code, _ := b.rpc(second, list); code != 200 {
			t.Errorf("the new token answered %d", code)
		}
		var kept string
		b.front.store.db.QueryRowContext(t.Context(), `SELECT digest FROM paypal_agent_tokens WHERE org = ?`, b.org).Scan(&kept)
		if kept == second || kept != digest(second) {
			t.Errorf("only a digest of the token may be kept")
		}
	})
}

func TestAnAgentReadsItsOwnOrganizationsBillingAndNobodyElses(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		_, other, err := b.front.store.SignUp(t.Context(), Signup{Email: "eve@globex.com", OrgName: "Globex"})
		if err != nil {
			t.Fatal(err)
		}
		mine, theirs := b.token(), ""
		if theirs, err = b.front.store.IssueAgentToken(other.Slug); err != nil {
			t.Fatal(err)
		}
		_, answer := b.rpc(mine, call("billing_status", `{}`))
		if text := answer.Result.Content[0].Text; !strings.Contains(text, `"state":"active"`) || !strings.Contains(text, `"seats_paid":1`) || !strings.Contains(text, `"price_per_seat_usd_a_month":"25.00"`) {
			t.Errorf("%s", text)
		}
		_, answer = b.rpc(theirs, call("billing_status", `{}`))
		if text := answer.Result.Content[0].Text; !strings.Contains(text, `"state":"not_subscribed"`) {
			t.Errorf("another organization's agent saw %s", text)
		}
	})
}

func TestAnAgentOnlyProposesAndNothingChangesUntilAPersonApproves(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		token := b.token()
		code, answer := b.rpc(token, call("billing_propose_seats", `{"seats":3,"reason":"Two colleagues start Monday"}`))
		if code != 200 || answer.Result.IsError || !strings.Contains(answer.Result.Content[0].Text, "Nothing has changed") {
			t.Fatalf("%d %+v", code, answer)
		}
		open, _ := b.front.store.OpenProposals(b.org)
		if len(open) != 1 || open[0].Source != "agent" || open[0].Seats != 3 || open[0].Status != ProposalPending {
			t.Fatalf("%+v", open)
		}
		if b.record().Seats != 1 || b.quota().Members != 1 || b.fake.count("POST /v1/billing/subscriptions/"+id+"/revise") != 0 {
			t.Errorf("a proposal changed something or reached PayPal: %+v", b.record())
		}
		_, again := b.rpc(token, call("billing_propose_seats", `{"seats":4,"reason":"more"}`))
		if !again.Result.IsError || !strings.Contains(again.Result.Content[0].Text, "already waiting") {
			t.Errorf("a second proposal: %+v", again)
		}
		_, page := b.admin.get("/org")
		if !strings.Contains(page, "Two colleagues start Monday") {
			t.Errorf("the admin cannot see the reason:\n%s", page)
		}
	})
}

func TestAnAgentIsToldWhyAnAskIsRefusedAndHasNoToolToApproveOrPay(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		token := b.token()
		for name, body := range map[string]string{
			"no subscription": call("billing_propose_seats", `{"seats":3,"reason":"x"}`),
			"bad arguments":   call("billing_propose_seats", `{"seats":"many"}`),
			"no reason":       call("billing_propose_seats", `{"seats":3,"reason":"  "}`),
			"unknown tool":    call("billing_approve", `{}`),
		} {
			if _, answer := b.rpc(token, body); !answer.Result.IsError || len(answer.Result.Content) == 0 {
				t.Errorf("%s was not an error: %+v", name, answer)
			}
		}
		_, answer := b.rpc(token, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		for _, tool := range answer.Result.Tools {
			if strings.Contains(tool.Name, "approve") || strings.Contains(tool.Name, "pay") || strings.Contains(tool.Name, "subscribe") {
				t.Errorf("an agent has the tool %s", tool.Name)
			}
		}
	})
}

func TestTheAgentEndpointAnswersTheHandshakeAndAcceptsNotifications(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		token := b.token()
		code, answer := b.rpc(token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		if code != 200 || answer.Error != nil {
			t.Errorf("%d %+v", code, answer)
		}
		if code, _ := b.rpc(token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != 202 {
			t.Errorf("a notification answered %d", code)
		}
		if _, answer := b.rpc(token, `{"jsonrpc":"2.0","id":3,"method":"resources/list"}`); answer.Error == nil || answer.Error.Code != -32601 {
			t.Errorf("an unknown method: %+v", answer)
		}
	})
}
