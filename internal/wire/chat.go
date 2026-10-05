package wire

import (
	"encoding/json"
	"strings"
)

// chatRequest is a call in Chat Completions, as the translators make it.
type chatRequest struct {
	Model             string         `json:"model"`
	Messages          []chatMessage  `json:"messages"`
	Tools             []chatTool     `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	MaxTokens         *int           `json:"max_tokens,omitempty"`
	Temperature       *float64       `json:"temperature,omitempty"`
	TopP              *float64       `json:"top_p,omitempty"`
	Stop              []string       `json:"stop,omitempty"`
	Stream            bool           `json:"stream,omitempty"`
	StreamOptions     *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    chatContent    `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// chatContent is a message's text, with any images. It is written as one string when there are no images, which every
// provider takes, and as parts when there are; an assistant message that only calls tools has none, written as null.
type chatContent struct {
	texts  []string
	images []string
}

func (c chatContent) MarshalJSON() ([]byte, error) {
	if len(c.images) == 0 {
		if len(c.texts) == 0 {
			return []byte("null"), nil
		}
		return json.Marshal(strings.Join(c.texts, "\n\n"))
	}
	parts := []map[string]any{}
	for _, text := range c.texts {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, url := range c.images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}})
	}
	return json.Marshal(parts)
}

func (c chatContent) empty() bool { return len(c.texts) == 0 && len(c.images) == 0 }

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func toolCall(id, name, arguments string) chatToolCall {
	call := chatToolCall{ID: id, Type: "function"}
	call.Function.Name, call.Function.Arguments = name, arguments
	return call
}

// noSchema is the parameters of a tool that declared none, since some providers refuse a function without them.
var noSchema = json.RawMessage(`{"type":"object","properties":{}}`)

// tidy makes a conversation one that the strictest Chat Completions template takes. All instructions given before the
// conversation began become the one system message at its start, and one given in the middle of it becomes a user's
// message, since many open models' templates allow a system message only first. Users' messages next to each other
// become one. Every tool call is answered before the conversation goes on, with a note where the harness sent no
// result, because a provider refuses a call left unanswered.
func tidy(messages []chatMessage) []chatMessage {
	out := []chatMessage{}
	var system chatContent
	started := false
	pending := []string{}
	answer := func() {
		for _, id := range pending {
			out = append(out, chatMessage{Role: "tool", ToolCallID: id, Content: chatContent{texts: []string{"(no result)"}}})
		}
		pending = nil
	}
	for _, message := range messages {
		if message.Role == "system" && !started {
			system.texts = append(system.texts, message.Content.texts...)
			continue
		}
		started = true
		if message.Role == "tool" {
			pending = without(pending, message.ToolCallID)
			out = append(out, message)
			continue
		}
		answer()
		if message.Role == "system" {
			message.Role = "user"
		}
		if message.Content.empty() && len(message.ToolCalls) == 0 {
			continue
		}
		if last := len(out) - 1; message.Role == "user" && last >= 0 && out[last].Role == "user" {
			out[last].Content.texts = append(out[last].Content.texts, message.Content.texts...)
			out[last].Content.images = append(out[last].Content.images, message.Content.images...)
			continue
		}
		for _, call := range message.ToolCalls {
			pending = append(pending, call.ID)
		}
		out = append(out, message)
	}
	answer()
	if !system.empty() {
		out = append([]chatMessage{{Role: "system", Content: system}}, out...)
	}
	return out
}

func without(ids []string, id string) []string {
	kept := ids[:0]
	for _, each := range ids {
		if each != id {
			kept = append(kept, each)
		}
	}
	return kept
}

// Options is what a credential says about the calls translated for it.
type Options struct {
	// MaxOutput is the most a provider lets a model write in one answer, or 0 when a request is not to be held to
	// one. A harness asks for what its own provider allows, which another provider can refuse outright.
	MaxOutput int
}

func (o Options) limit(asked *int) *int {
	if asked == nil || o.MaxOutput <= 0 || *asked <= o.MaxOutput {
		return asked
	}
	held := o.MaxOutput
	return &held
}

// finish is the request every translator ends with: the conversation tidied, and a streamed one asked to report its
// usage, since a Chat Completions stream says nothing about usage unless asked.
func (c *chatRequest) finish() {
	c.Messages = tidy(c.Messages)
	if c.Stream {
		c.StreamOptions = &streamOptions{IncludeUsage: true}
	}
}
