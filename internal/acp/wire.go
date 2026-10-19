package acp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// message is any JSON-RPC line: a request carries a method and an id, a
// notification a method alone, and a response an id alone.
type message struct {
	ID     *json.RawMessage `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// conn is the client end of one agent's stdio. Everything the agent says is
// handed to keep, except while muted: a loaded session replays its history,
// and that history belongs to the turns that already recorded it.
type conn struct {
	in    io.Writer
	lines *bufio.Scanner
	keep  func(line string) error
	muted bool
	next  int
}

func newConn(in io.Writer, out io.Reader, keep func(string) error) *conn {
	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &conn{in: in, lines: lines, keep: keep}
}

// call sends one request and reads until its answer, serving whatever the agent
// asks of the client on the way.
func (c *conn) call(method string, params any) (json.RawMessage, error) {
	c.next++
	id := c.next
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for c.lines.Scan() {
		line := c.lines.Text()
		var got message
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			if err := c.kept(line); err != nil {
				return nil, err
			}
			continue
		}
		if got.Method != "" && got.ID != nil {
			if err := c.serve(got); err != nil {
				return nil, err
			}
			continue
		}
		if err := c.kept(line); err != nil {
			return nil, err
		}
		if got.Method == "" && got.ID != nil && string(*got.ID) == fmt.Sprint(id) {
			if got.Error != nil {
				return nil, fmt.Errorf("%s: %w", method, got.Error)
			}
			return got.Result, nil
		}
	}
	if err := c.lines.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%s: %w", method, errClosed)
}

var errClosed = errors.New("the agent stopped before it answered")

func (c *conn) kept(line string) error {
	if c.muted {
		return nil
	}
	return c.keep(line)
}

// serve answers a request from the agent. Permission is granted because the
// sandbox is the boundary: a turn nobody is watching has nobody to ask. The
// client offers no files and no terminal, so anything else is refused.
func (c *conn) serve(asked message) error {
	reply := map[string]any{"jsonrpc": "2.0", "id": asked.ID}
	if asked.Method == "session/request_permission" {
		reply["result"] = map[string]any{"outcome": permit(asked.Params)}
	} else {
		reply["error"] = rpcError{Code: -32601, Message: "method not found"}
	}
	return c.send(reply)
}

// permit picks the broadest allowing option the agent offered.
func permit(params json.RawMessage) map[string]any {
	var asked struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	_ = json.Unmarshal(params, &asked)
	for _, kind := range []string{"allow_always", "allow_once"} {
		for _, option := range asked.Options {
			if option.Kind == kind {
				return map[string]any{"outcome": "selected", "optionId": option.OptionID}
			}
		}
	}
	return map[string]any{"outcome": "cancelled"}
}

func (c *conn) send(value any) error {
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(line, '\n'))
	return err
}
