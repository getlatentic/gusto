package wire

import (
	"encoding/json"
	"fmt"
	"strings"
)

// messagesRequest is a call in Anthropic's Messages API, as Claude Code makes one. The fields it sends that a Chat
// Completions provider has no use for, such as thinking, cache control and context management, are not read.
type messagesRequest struct {
	Model    string          `json:"model"`
	System   json.RawMessage `json:"system"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Type        string          `json:"type"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	} `json:"tools"`
	ToolChoice *struct {
		Type                   string `json:"type"`
		Name                   string `json:"name"`
		DisableParallelToolUse bool   `json:"disable_parallel_tool_use"`
	} `json:"tool_choice"`
	MaxTokens     *int     `json:"max_tokens"`
	Temperature   *float64 `json:"temperature"`
	TopP          *float64 `json:"top_p"`
	StopSequences []string `json:"stop_sequences"`
	Stream        bool     `json:"stream"`
}

// block is one piece of a message's content.
type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source"`
}

// blocksOf reads content that is either a string or a list of blocks.
func blocksOf(raw json.RawMessage) ([]block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []block{{Type: "text", Text: text}}, nil
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("content is neither text nor a list of blocks: %w", err)
	}
	return blocks, nil
}

// image is a picture as a data or web address, the form Chat Completions takes.
func (b block) image() string {
	if b.Source == nil {
		return ""
	}
	if b.Source.Type == "url" {
		return b.Source.URL
	}
	if b.Source.Data == "" {
		return ""
	}
	return "data:" + b.Source.MediaType + ";base64," + b.Source.Data
}

// fromMessages is a Messages call as a Chat Completions one.
func fromMessages(body []byte, options Options) (chatRequest, error) {
	var asked messagesRequest
	if err := json.Unmarshal(body, &asked); err != nil {
		return chatRequest{}, fmt.Errorf("that is not a call to the Messages API: %w", err)
	}
	out := chatRequest{Model: asked.Model, MaxTokens: options.limit(asked.MaxTokens), Temperature: asked.Temperature,
		TopP: asked.TopP, Stop: asked.StopSequences, Stream: asked.Stream}
	system, err := blocksOf(asked.System)
	if err != nil {
		return chatRequest{}, fmt.Errorf("system: %w", err)
	}
	if instructions := textOf(system); len(instructions) > 0 {
		out.Messages = append(out.Messages, chatMessage{Role: "system", Content: chatContent{texts: instructions}})
	}
	for i, message := range asked.Messages {
		blocks, err := blocksOf(message.Content)
		if err != nil {
			return chatRequest{}, fmt.Errorf("message %d: %w", i, err)
		}
		out.Messages = append(out.Messages, messagesTurn(message.Role, blocks)...)
	}
	for _, tool := range asked.Tools {
		// A tool with a type of its own, such as web search, is one Anthropic runs; no other provider can.
		if tool.Type != "" && tool.Type != "custom" {
			continue
		}
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = noSchema
		}
		out.Tools = append(out.Tools, chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}})
	}
	if choice := asked.ToolChoice; choice != nil && len(out.Tools) > 0 {
		out.ToolChoice = map[string]any{"auto": "auto", "any": "required", "none": "none"}[choice.Type]
		if choice.Type == "tool" {
			out.ToolChoice = map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}}
		}
		if choice.DisableParallelToolUse {
			parallel := false
			out.ParallelToolCalls = &parallel
		}
	}
	out.finish()
	return out, nil
}

// messagesTurn is one message as the Chat Completions messages it becomes. A user's tool results come first, each a
// message of its own, since Chat Completions answers a tool call with one, and what the user said with them follows.
// An assistant's thinking is left out: it is the provider's own, and another provider cannot read it back.
func messagesTurn(role string, blocks []block) []chatMessage {
	said := chatMessage{Role: role}
	var results []chatMessage
	for _, piece := range blocks {
		switch piece.Type {
		case "text":
			if piece.Text != "" {
				said.Content.texts = append(said.Content.texts, piece.Text)
			}
		case "image":
			if url := piece.image(); url != "" {
				said.Content.images = append(said.Content.images, url)
			}
		case "tool_use":
			arguments := string(piece.Input)
			if strings.TrimSpace(arguments) == "" || arguments == "null" {
				arguments = "{}"
			}
			said.ToolCalls = append(said.ToolCalls, toolCall(piece.ID, piece.Name, arguments))
		case "tool_result":
			results = append(results, chatMessage{Role: "tool", ToolCallID: piece.ToolUseID, Content: chatContent{texts: []string{resultText(piece)}}})
		}
	}
	return append(results, said)
}

// resultText is a tool's result as the text Chat Completions carries. A picture in it cannot go back as a tool's
// result there, so the result says one was left out.
func resultText(piece block) string {
	inner, err := blocksOf(piece.Content)
	if err != nil {
		return string(piece.Content)
	}
	parts := textOf(inner)
	for _, each := range inner {
		if each.Type == "image" {
			parts = append(parts, "(an image was left out)")
		}
	}
	text := strings.Join(parts, "\n")
	if piece.IsError {
		text = "Error: " + text
	}
	if text == "" {
		text = "(no output)"
	}
	return text
}

func textOf(blocks []block) []string {
	var texts []string
	for _, piece := range blocks {
		if piece.Type == "text" && piece.Text != "" {
			texts = append(texts, piece.Text)
		}
	}
	return texts
}
