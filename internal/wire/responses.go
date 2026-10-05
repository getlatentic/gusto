package wire

import (
	"encoding/json"
	"fmt"
	"strings"
)

// responsesRequest is a call in OpenAI's Responses API, as Codex makes one. It is sent with store off and the whole
// conversation in it, which is the only way Codex calls a provider other than OpenAI, so nothing is kept between calls.
type responsesRequest struct {
	Model             string            `json:"model"`
	Instructions      string            `json:"instructions"`
	Input             json.RawMessage   `json:"input"`
	Tools             []json.RawMessage `json:"tools"`
	ToolChoice        json.RawMessage   `json:"tool_choice"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls"`
	MaxOutputTokens   *int              `json:"max_output_tokens"`
	Temperature       *float64          `json:"temperature"`
	TopP              *float64          `json:"top_p"`
	Stream            bool              `json:"stream"`
}

// item is one entry of a Responses conversation: a message, a tool call or a tool's output.
type item struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Output    json.RawMessage `json:"output"`
}

type part struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url"`
}

// contentOf reads content that is either a string or a list of parts.
func contentOf(raw json.RawMessage) (chatContent, error) {
	var content chatContent
	if len(raw) == 0 || string(raw) == "null" {
		return content, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if text != "" {
			content.texts = []string{text}
		}
		return content, nil
	}
	var parts []part
	if err := json.Unmarshal(raw, &parts); err != nil {
		return content, fmt.Errorf("content is neither text nor a list of parts: %w", err)
	}
	for _, piece := range parts {
		switch piece.Type {
		case "input_text", "output_text", "text":
			if piece.Text != "" {
				content.texts = append(content.texts, piece.Text)
			}
		case "input_image":
			if piece.ImageURL != "" {
				content.images = append(content.images, piece.ImageURL)
			}
		}
	}
	return content, nil
}

// fromResponses is a Responses call as a Chat Completions one, and the names of the tools Codex calls with free text,
// which have to go back to it as free text.
func fromResponses(body []byte, options Options) (chatRequest, map[string]bool, error) {
	var asked responsesRequest
	if err := json.Unmarshal(body, &asked); err != nil {
		return chatRequest{}, nil, fmt.Errorf("that is not a call to the Responses API: %w", err)
	}
	out := chatRequest{Model: asked.Model, MaxTokens: options.limit(asked.MaxOutputTokens), Temperature: asked.Temperature,
		TopP: asked.TopP, ParallelToolCalls: asked.ParallelToolCalls, Stream: asked.Stream}
	if asked.Instructions != "" {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: chatContent{texts: []string{asked.Instructions}}})
	}
	items, err := itemsOf(asked.Input)
	if err != nil {
		return chatRequest{}, nil, err
	}
	for i, each := range items {
		if err := addItem(&out, each); err != nil {
			return chatRequest{}, nil, fmt.Errorf("input %d: %w", i, err)
		}
	}
	custom := map[string]bool{}
	for _, raw := range asked.Tools {
		if tool, freeText, ok := responsesTool(raw); ok {
			out.Tools = append(out.Tools, tool)
			custom[tool.Function.Name] = freeText
		}
	}
	out.ToolChoice = responsesChoice(asked.ToolChoice)
	if len(out.Tools) == 0 {
		out.ToolChoice, out.ParallelToolCalls = nil, nil
	}
	out.finish()
	return out, custom, nil
}

func itemsOf(raw json.RawMessage) ([]item, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []item{{Type: "message", Role: "user", Content: raw}}, nil
	}
	var items []item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input is neither text nor a list of items: %w", err)
	}
	return items, nil
}

// addItem puts one entry on the conversation. A tool call joins the assistant's message it follows, since Chat
// Completions has an assistant say and call in one message. What Codex keeps for OpenAI alone, its reasoning and its
// compacted history, is left out.
func addItem(out *chatRequest, each item) error {
	switch {
	case each.Type == "message" || (each.Type == "" && each.Role != ""):
		content, err := contentOf(each.Content)
		if err != nil {
			return err
		}
		role := each.Role
		if role == "developer" {
			role = "system"
		}
		out.Messages = append(out.Messages, chatMessage{Role: role, Content: content})
	case each.Type == "function_call" || each.Type == "custom_tool_call":
		arguments := each.Arguments
		if each.Type == "custom_tool_call" {
			wrapped, _ := json.Marshal(map[string]string{"input": each.Input})
			arguments = string(wrapped)
		}
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		call := toolCall(each.CallID, each.Name, arguments)
		if last := len(out.Messages) - 1; last >= 0 && out.Messages[last].Role == "assistant" {
			out.Messages[last].ToolCalls = append(out.Messages[last].ToolCalls, call)
			return nil
		}
		out.Messages = append(out.Messages, chatMessage{Role: "assistant", ToolCalls: []chatToolCall{call}})
	case each.Type == "function_call_output" || each.Type == "custom_tool_call_output":
		content, err := contentOf(each.Output)
		if err != nil {
			return err
		}
		text := strings.Join(content.texts, "\n")
		if len(content.images) > 0 {
			text += "\n(an image was left out)"
		}
		if strings.TrimSpace(text) == "" {
			text = "(no output)"
		}
		out.Messages = append(out.Messages, chatMessage{Role: "tool", ToolCallID: each.CallID, Content: chatContent{texts: []string{text}}})
	}
	return nil
}

// responsesTool is a tool as a Chat Completions function. A tool Codex calls with free text, such as a patch, becomes a
// function taking that text as its one argument. A tool OpenAI runs itself, such as web search, and a namespace of
// tools, which Chat Completions has no form for, are left out.
func responsesTool(raw json.RawMessage) (chatTool, bool, bool) {
	var tool struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	}
	if json.Unmarshal(raw, &tool) != nil {
		return chatTool{}, false, false
	}
	switch tool.Type {
	case "function":
		schema := tool.Parameters
		if len(schema) == 0 || string(schema) == "null" {
			schema = noSchema
		}
		return chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}}, false, true
	case "custom":
		schema := json.RawMessage(`{"type":"object","properties":{"input":{"type":"string","description":"The whole input, as text."}},"required":["input"]}`)
		return chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}}, true, true
	}
	return chatTool{}, false, false
}

func responsesChoice(raw json.RawMessage) any {
	var named string
	if json.Unmarshal(raw, &named) == nil {
		switch named {
		case "auto", "none", "required":
			return named
		}
		return nil
	}
	var choice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &choice) == nil && choice.Type == "function" && choice.Name != "" {
		return map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}}
	}
	return nil
}
