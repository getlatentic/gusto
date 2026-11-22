package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

// workingAgent is an agent that works on the prompt until it is told to cancel: it then asks one more permission, which a
// cancelled client must refuse, and ends the prompt as cancelled.
func workingAgent(t *testing.T, requests io.Reader, replies io.WriteCloser, heard chan<- string) {
	defer replies.Close()
	say := func(line string) { fmt.Fprintln(replies, line) }
	lines := bufio.NewScanner(requests)
	var prompt json.RawMessage
	for lines.Scan() {
		var got struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				SessionID string `json:"sessionId"`
			} `json:"params"`
			Result struct {
				Outcome struct {
					Outcome string `json:"outcome"`
				} `json:"outcome"`
			} `json:"result"`
		}
		if err := json.Unmarshal(lines.Bytes(), &got); err != nil {
			t.Errorf("the client sent a line that is not JSON: %s", lines.Text())
			return
		}
		switch got.Method {
		case "initialize":
			say(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, got.ID, dshCapabilities))
		case "session/new":
			say(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"s1"}}`, got.ID))
		case "session/prompt":
			prompt = got.ID
			say(chunk)
			heard <- "working"
		case "session/cancel":
			heard <- "cancel " + got.Params.SessionID
			say(`{"jsonrpc":"2.0","id":"p2","method":"session/request_permission","params":{"sessionId":"s1","options":[{"optionId":"once","kind":"allow_once"}]}}`)
		case "":
			heard <- "permission " + got.Result.Outcome.Outcome
			say(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"cancelled"}}`, prompt))
		}
	}
}

func TestStoppingATurnAsksTheAgentToCancelAndTheTurnEndsWhenItHas(t *testing.T) {
	toAgent, fromClient := io.Pipe()
	fromAgent, toClient := io.Pipe()
	heard := make(chan string, 8)
	go workingAgent(t, toAgent, toClient, heard)
	stop := make(chan struct{})
	type result struct {
		ended Ended
		err   error
	}
	done := make(chan result, 1)
	go func() {
		ended, err := Converse(fromClient, fromAgent, Turn{Cwd: "/w", Brief: "write the policy", Stop: stop}, func(string) error { return nil })
		done <- result{ended, err}
	}()
	if got := <-heard; got != "working" {
		t.Fatalf("the agent heard %q before it was stopped", got)
	}
	close(stop)
	for _, want := range []string{"cancel s1", "permission cancelled"} {
		select {
		case got := <-heard:
			if got != want {
				t.Fatalf("the agent heard %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the agent never heard %q", want)
		}
	}
	select {
	case got := <-done:
		if got.err != nil || got.ended != (Ended{Session: "s1", Stop: "cancelled"}) || got.ended.Answered() {
			t.Errorf("the turn ended %+v, %v; want it cancelled in session s1", got.ended, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not end once the agent cancelled it")
	}
	fromClient.Close()
}

func TestATurnThatEndsBeforeItIsStoppedSendsNoCancel(t *testing.T) {
	a := &agent{
		capabilities: dshCapabilities,
		answers: map[string]string{
			"session/new":    `"result":{"sessionId":"s1"}`,
			"session/prompt": `"result":{"stopReason":"end_turn"}`,
		},
	}
	stop := make(chan struct{})
	if _, _, err := run(t, a, Turn{Cwd: "/w", Brief: "hi", Stop: stop}); err != nil {
		t.Fatal(err)
	}
	close(stop)
	time.Sleep(50 * time.Millisecond)
	for _, method := range a.asked {
		if method == "session/cancel" {
			t.Errorf("a turn already over was cancelled: %v", a.asked)
		}
	}
}
