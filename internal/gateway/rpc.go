package gateway

import "encoding/json"

// JSON-RPC 2.0 as the Model Context Protocol uses it.

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// notification says whether a message wants no answer: it has no id.
func (r request) notification() bool { return len(r.ID) == 0 }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

const (
	codeParse    = -32700
	codeInvalid  = -32600
	codeMethod   = -32601
	codeParams   = -32602
	codeInternal = -32603
)

// protocols are the versions of the protocol this speaks, newest first. A client that asks for one is answered with it, and one
// that asks for another is answered with the newest.
var protocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

func agreed(asked string) string {
	for _, version := range protocols {
		if version == asked {
			return version
		}
	}
	return protocols[0]
}

// Tool is a tool as a server describes it.
type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// Content is one part of what a tool answered.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Other fields of a part (an image's data, a resource) are kept as they came.
	Rest map[string]json.RawMessage `json:"-"`
}

// Result is what a tool call answers.
type Result struct {
	Content           []json.RawMessage `json:"content"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

func text(message string, isError bool) Result {
	part, _ := json.Marshal(map[string]string{"type": "text", "text": message})
	return Result{Content: []json.RawMessage{part}, IsError: isError}
}
