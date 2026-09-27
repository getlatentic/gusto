package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

// agent plays an ACP server: it answers each method with what the script says,
// saying the script's notifications first. Its capabilities line is the one
// dsh 0.1.5 sends, which resumes and cannot load.
type agent struct {
	capabilities string
	before       map[string][]string
	answers      map[string]string
	asked        []string
	granted      string
}

const dshCapabilities = `{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"close":{},"list":{},"resume":{}}},"authMethods":[]}`

func (a *agent) serve(t *testing.T, requests io.Reader, pipe io.WriteCloser) {
	// A pipe between processes has a buffer and io.Pipe has none, so the agent
	// queues what it says rather than waiting on a client that is itself
	// writing an answer to the agent.
	queued := make(chan string, 64)
	defer close(queued)
	go func() {
		for line := range queued {
			fmt.Fprint(pipe, line)
		}
		pipe.Close()
	}()
	replies := queueWriter(queued)
	lines := bufio.NewScanner(requests)
	for lines.Scan() {
		var got struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result struct {
				Outcome struct {
					OptionID string `json:"optionId"`
				} `json:"outcome"`
			} `json:"result"`
		}
		if err := json.Unmarshal(lines.Bytes(), &got); err != nil {
			t.Errorf("the client sent a line that is not JSON: %s", lines.Text())
			return
		}
		if got.Method == "" {
			a.granted = got.Result.Outcome.OptionID
			continue
		}
		a.asked = append(a.asked, got.Method)
		for _, said := range a.before[got.Method] {
			fmt.Fprintln(replies, said)
		}
		answer := a.answers[got.Method]
		if got.Method == "initialize" {
			answer = `"result":` + a.capabilities
		}
		if answer == "" {
			answer = `"result":{}`
		}
		fmt.Fprintf(replies, `{"jsonrpc":"2.0","id":%s,%s}`+"\n", got.ID, answer)
	}
}

type queueWriter chan<- string

func (q queueWriter) Write(line []byte) (int, error) {
	q <- string(line)
	return len(line), nil
}

func run(t *testing.T, a *agent, turn Turn) (Ended, []string, error) {
	t.Helper()
	toAgent, fromClient := io.Pipe()
	fromAgent, toClient := io.Pipe()
	served := make(chan struct{})
	go func() {
		a.serve(t, toAgent, toClient)
		close(served)
	}()
	var kept []string
	ended, err := Converse(fromClient, fromAgent, turn, func(line string) error {
		kept = append(kept, line)
		return nil
	})
	fromClient.Close()
	<-served
	return ended, kept, err
}

const (
	chunk      = `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Fixed."}}}}`
	permission = `{"jsonrpc":"2.0","id":"p1","method":"session/request_permission","params":{"sessionId":"s1","options":[{"optionId":"no","kind":"reject_once"},{"optionId":"once","kind":"allow_once"},{"optionId":"always","kind":"allow_always"}]}}`
)

func TestANewConversationKeepsWhatTheAgentSaidAndItsSession(t *testing.T) {
	a := &agent{
		capabilities: dshCapabilities,
		before:       map[string][]string{"session/prompt": {permission, chunk}},
		answers: map[string]string{
			"session/new":    `"result":{"sessionId":"s1"}`,
			"session/prompt": `"result":{"stopReason":"end_turn"}`,
		},
	}
	ended, kept, err := run(t, a, Turn{Cwd: "/job/workspace", Brief: "fix it"})
	if err != nil {
		t.Fatal(err)
	}
	if ended != (Ended{Session: "s1", Stop: "end_turn"}) {
		t.Errorf("ended = %+v", ended)
	}
	if a.granted != "always" {
		t.Errorf("granted %q, want the broadest allowing option", a.granted)
	}
	if !strings.Contains(strings.Join(kept, "\n"), "Fixed.") {
		t.Error("the agent's words should reach the record")
	}
	for _, line := range kept {
		if strings.Contains(line, "request_permission") {
			t.Error("a request to the client is answered, not recorded as the agent's output")
		}
	}
}

func TestARememberedSessionIsResumedRatherThanStartedAgain(t *testing.T) {
	a := &agent{
		capabilities: dshCapabilities,
		before:       map[string][]string{"session/resume": {chunk}},
		answers:      map[string]string{"session/prompt": `"result":{"stopReason":"end_turn"}`},
	}
	ended, kept, err := run(t, a, Turn{Cwd: "/job/workspace", Brief: "and then?", Session: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(a.asked, " "); got != "initialize session/resume session/prompt" {
		t.Errorf("asked %s", got)
	}
	if ended.Session != "s1" {
		t.Errorf("session = %q", ended.Session)
	}
	if strings.Contains(strings.Join(kept, "\n"), "Fixed.") {
		t.Error("hermes replays the history on resume, and that belongs to earlier turns")
	}
}

func TestALoadedSessionsReplayIsNotRecordedAgain(t *testing.T) {
	a := &agent{
		capabilities: `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`,
		before:       map[string][]string{"session/load": {chunk}},
		answers:      map[string]string{"session/prompt": `"result":{"stopReason":"end_turn"}`},
	}
	_, kept, err := run(t, a, Turn{Cwd: "/w", Brief: "and then?", Session: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(a.asked, " "); got != "initialize session/load session/prompt" {
		t.Errorf("asked %s", got)
	}
	if strings.Contains(strings.Join(kept, "\n"), "Fixed.") {
		t.Error("history the agent replays belongs to the turns that already recorded it")
	}
}

func TestAFailedResumeKeepsTheThreadsSession(t *testing.T) {
	a := &agent{capabilities: dshCapabilities, answers: map[string]string{
		"session/resume": `"error":{"code":-32602,"message":"session cwd does not match"}`,
	}}
	ended, _, err := run(t, a, Turn{Cwd: "/w", Brief: "and then?", Session: "s1"})
	if err == nil || !strings.Contains(err.Error(), "cwd does not match") {
		t.Fatalf("err = %v, want the agent's reason", err)
	}
	if ended.Session != "s1" {
		t.Error("one failed turn should not cost the thread its memory")
	}
}

func TestAHarnessThatStopsMidTurnIsAFailure(t *testing.T) {
	toAgent, fromClient := io.Pipe()
	fromAgent, toClient := io.Pipe()
	go func() {
		_, _ = bufio.NewReader(toAgent).ReadString('\n')
		toClient.Close()
	}()
	_, err := Converse(fromClient, fromAgent, Turn{Brief: "x"}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), errClosed.Error()) {
		t.Errorf("err = %v", err)
	}
}
