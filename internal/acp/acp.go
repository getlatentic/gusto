// Package acp holds one turn of a conversation with a harness that speaks the
// Agent Client Protocol: JSON-RPC over the harness's standard input and output.
//
// A harness that only answers one task and exits cannot continue a
// conversation. Over ACP the client names the session to load, so the same
// harness can keep a thread going across turns.
package acp

import (
	"encoding/json"
	"io"
)

// Turn is one message in a conversation.
type Turn struct {
	Cwd   string
	Brief string
	// Session is the harness's name for the conversation to continue. Empty
	// starts a new one.
	Session string
}

// Ended is how the turn finished: the session it belongs to, which the next
// turn names, and the harness's reason for stopping.
type Ended struct {
	Session string
	Stop    string
}

// Converse runs one turn. Every line the harness says during it goes to keep,
// with the answers to haas's own requests among them, so the record holds the
// session id and the usage exactly as the harness reported them.
func Converse(in io.Writer, out io.Reader, turn Turn, keep func(line string) error) (Ended, error) {
	c := newConn(in, out, keep)
	can, err := initialize(c)
	if err != nil {
		return Ended{}, err
	}
	session, err := open(c, turn, can)
	if err != nil {
		return Ended{Session: session}, err
	}
	answer, err := c.call("session/prompt", map[string]any{
		"sessionId": session,
		"prompt":    []map[string]string{{"type": "text", "text": turn.Brief}},
	})
	if err != nil {
		return Ended{Session: session}, err
	}
	var ended struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(answer, &ended)
	return Ended{Session: session, Stop: ended.StopReason}, nil
}

// continuing is how an agent can take up a session it held before. Resuming
// carries on without replaying the history, and loading replays it first.
type continuing struct {
	resume, load bool
}

func initialize(c *conn) (continuing, error) {
	answer, err := c.call("initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]string{"name": "haas"},
	})
	if err != nil {
		return continuing{}, err
	}
	var agent struct {
		AgentCapabilities struct {
			LoadSession         bool `json:"loadSession"`
			SessionCapabilities struct {
				Resume json.RawMessage `json:"resume"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	_ = json.Unmarshal(answer, &agent)
	resume := agent.AgentCapabilities.SessionCapabilities.Resume
	return continuing{
		resume: len(resume) > 0 && string(resume) != "null",
		load:   agent.AgentCapabilities.LoadSession,
	}, nil
}

// open takes up the named session when the agent can, and starts a new one
// otherwise. A named session the agent cannot find fails the turn rather than
// quietly starting over, which would lose the thread without saying so.
func open(c *conn, turn Turn, can continuing) (string, error) {
	held := map[string]any{"sessionId": turn.Session, "cwd": turn.Cwd, "mcpServers": []any{}}
	switch {
	case turn.Session != "" && can.resume:
		_, err := c.call("session/resume", held)
		return turn.Session, err
	case turn.Session != "" && can.load:
		c.muted = true
		_, err := c.call("session/load", held)
		c.muted = false
		return turn.Session, err
	}
	answer, err := c.call("session/new", map[string]any{"cwd": turn.Cwd, "mcpServers": []any{}})
	if err != nil {
		return turn.Session, err
	}
	var opened struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(answer, &opened); err != nil {
		return "", err
	}
	return opened.SessionID, nil
}
