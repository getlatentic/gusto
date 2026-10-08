package gateway

import (
	"context"
	"encoding/json"
	"fmt"
)

// exchange is one request to a server and its result, however the server is reached.
type exchange func(ctx context.Context, method string, params any) (json.RawMessage, error)

// Conn is a connection to one MCP server, whether it is an address or a program.
type Conn interface {
	Tools(ctx context.Context) ([]Tool, error)
	Call(ctx context.Context, tool string, arguments json.RawMessage) (Result, error)
	Close()
}

// Tools is every tool the server has, all its pages.
func listTools(ctx context.Context, server string, call exchange) ([]Tool, error) {
	var all []Tool
	cursor := ""
	for range 50 {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var page struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("%s answered tools/list with something that is not a list: %w", server, err)
		}
		all = append(all, page.Tools...)
		if cursor = page.NextCursor; cursor == "" {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%s lists more pages of tools than the gateway reads", server)
}

// Call calls one tool.
func callTool(ctx context.Context, server string, call exchange, tool string, arguments json.RawMessage) (Result, error) {
	params := map[string]any{"name": tool}
	if len(arguments) > 0 {
		params["arguments"] = arguments
	}
	raw, err := call(ctx, "tools/call", params)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return Result{}, fmt.Errorf("%s answered a call with something that is not a result: %w", server, err)
	}
	return result, nil
}
