package wire

import (
	"encoding/json"
	"sort"
)

// messagesAnswer writes an answer as Anthropic's Messages API gives one: as events when the call was streamed, and as
// one message at the end when it was not. Text is written as it arrives. A tool call is written whole once its
// arguments are, since the pieces of two calls can interleave in Chat Completions and cannot in a Messages stream.
type messagesAnswer struct {
	out      *events
	streamed bool
	id       string
	model    string
	content  []map[string]any
	block    int
	inText   bool
	written  string
	calls    map[int]*pendingCall
}

type pendingCall struct {
	id, name  string
	arguments string
}

func newMessagesAnswer(out *events, streamed bool) *messagesAnswer {
	return &messagesAnswer{out: out, streamed: streamed, id: newID("msg_"), calls: map[int]*pendingCall{}}
}

func (m *messagesAnswer) send(kind string, data map[string]any) error {
	if !m.streamed {
		return nil
	}
	data["type"] = kind
	return m.out.send(kind, data)
}

func (m *messagesAnswer) begin(model string) error {
	m.model = model
	return m.send("message_start", map[string]any{"message": map[string]any{
		"id": m.id, "type": "message", "role": "assistant", "model": model, "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": map[string]int64{"input_tokens": 0, "output_tokens": 0},
	}})
}

func (m *messagesAnswer) text(delta string) error {
	if err := m.flushCalls(); err != nil {
		return err
	}
	if !m.inText {
		m.inText, m.written = true, ""
		if err := m.send("content_block_start", map[string]any{"index": m.block, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
			return err
		}
	}
	m.written += delta
	return m.send("content_block_delta", map[string]any{"index": m.block, "delta": map[string]any{"type": "text_delta", "text": delta}})
}

func (m *messagesAnswer) closeText() error {
	if !m.inText {
		return nil
	}
	m.inText = false
	m.content = append(m.content, map[string]any{"type": "text", "text": m.written})
	err := m.send("content_block_stop", map[string]any{"index": m.block})
	m.block++
	return err
}

func (m *messagesAnswer) tool(index int, id, name string) error {
	if err := m.closeText(); err != nil {
		return err
	}
	m.calls[index] = &pendingCall{id: id, name: name}
	return nil
}

func (m *messagesAnswer) arguments(index int, delta string) error {
	if call, found := m.calls[index]; found {
		call.arguments += delta
	}
	return nil
}

// flushCalls writes the tool calls made so far, in the order the model made them.
func (m *messagesAnswer) flushCalls() error {
	indexes := make([]int, 0, len(m.calls))
	for index := range m.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		call := m.calls[index]
		input := json.RawMessage(call.arguments)
		if !json.Valid(input) {
			input = json.RawMessage("{}")
		}
		m.content = append(m.content, map[string]any{"type": "tool_use", "id": call.id, "name": call.name, "input": input})
		if err := m.send("content_block_start", map[string]any{"index": m.block, "content_block": map[string]any{"type": "tool_use", "id": call.id, "name": call.name, "input": map[string]any{}}}); err != nil {
			return err
		}
		if err := m.send("content_block_delta", map[string]any{"index": m.block, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(input)}}); err != nil {
			return err
		}
		if err := m.send("content_block_stop", map[string]any{"index": m.block}); err != nil {
			return err
		}
		m.block++
	}
	m.calls = map[int]*pendingCall{}
	return nil
}

// messagesStop is why the model stopped, in Anthropic's words.
var messagesStop = map[Stop]string{StopEnd: "end_turn", StopLength: "max_tokens", StopTools: "tool_use", StopFilter: "refusal"}

func (m *messagesAnswer) end(stop Stop, usage Usage) error {
	if err := m.closeText(); err != nil {
		return err
	}
	if err := m.flushCalls(); err != nil {
		return err
	}
	// Anthropic counts the input read from a cache apart from the rest of it.
	counted := map[string]int64{"input_tokens": usage.Input - usage.Cached, "output_tokens": usage.Output, "cache_read_input_tokens": usage.Cached}
	if !m.streamed {
		return m.out.whole(map[string]any{"id": m.id, "type": "message", "role": "assistant", "model": m.model,
			"content": m.contentOrEmpty(), "stop_reason": messagesStop[stop], "stop_sequence": nil, "usage": counted})
	}
	if err := m.send("message_delta", map[string]any{"delta": map[string]any{"stop_reason": messagesStop[stop], "stop_sequence": nil}, "usage": counted}); err != nil {
		return err
	}
	return m.send("message_stop", map[string]any{})
}

func (m *messagesAnswer) contentOrEmpty() []map[string]any {
	if m.content == nil {
		return []map[string]any{}
	}
	return m.content
}

// messagesError is an error in the body Anthropic's API gives one in, which Claude Code reads its message from.
func messagesError(status int, message string) map[string]any {
	kinds := map[int]string{400: "invalid_request_error", 401: "authentication_error", 403: "permission_error",
		404: "not_found_error", 413: "request_too_large", 429: "rate_limit_error", 529: "overloaded_error"}
	kind, found := kinds[status]
	if !found {
		kind = "api_error"
	}
	return map[string]any{"type": "error", "error": map[string]string{"type": kind, "message": message}}
}
