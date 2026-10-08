// Package bridge is the helper speaking the Agent Client Protocol to an editor: the editor starts the helper as its agent,
// talks to it over standard input and output, and the helper does what the editor asks in the person's cloud agent. It is the
// agent side of the protocol. The agent itself runs in the cloud, so the editor's file and terminal methods are not used.
package bridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// request is any JSON-RPC message from the editor: a request has an id and a method, a notification a method alone, and an
// answer to something the helper asked has an id alone.
type request struct {
	ID     *json.RawMessage `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC's own codes, and the protocol's for a person who is not signed in.
const (
	codeParse    = -32700
	codeInvalid  = -32600
	codeMethod   = -32601
	codeParams   = -32602
	codeInternal = -32603
	codeAuth     = -32000
)

// wire writes what the helper says to the editor, one message to a line. Many turns write at once, so a line is never
// mixed with another.
type wire struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *wire) send(message any) error {
	text, err := json.Marshal(message)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.out.Write(append(text, '\n'))
	return err
}

func (w *wire) answer(id *json.RawMessage, result any) error {
	return w.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (w *wire) fail(id *json.RawMessage, code int, message string) error {
	return w.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: message}})
}

func (w *wire) notify(method string, params any) error {
	return w.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// readRequests reads the editor's lines until it hangs up.
func readRequests(in io.Reader, handle func(request, error)) error {
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 0, 64<<10), 32<<20)
	for lines.Scan() {
		text := lines.Bytes()
		if len(text) == 0 {
			continue
		}
		var got request
		if err := json.Unmarshal(text, &got); err != nil {
			handle(request{}, fmt.Errorf("this is not JSON: %v", err))
			continue
		}
		handle(got, nil)
	}
	return lines.Err()
}
