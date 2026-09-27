package web

import (
	"encoding/json"
	"testing"
)

// What an agent did reads as what it did, whichever harness said it and in
// whatever shape.
func TestStepsAreSaidInWords(t *testing.T) {
	for _, tc := range []struct {
		done ran
		want step
	}{
		{ran{Tool: "skill", Input: json.RawMessage(`{"name":"keep-facts"}`)}, step{"Used skill", "Keep facts"}},
		{ran{Tool: "mcp__memory__search_nodes", Input: json.RawMessage(`{"query":"favourite colour"}`)}, step{"Memory", "search nodes: favourite colour"}},
		{ran{Tool: "mcp__memory__read_graph", Input: json.RawMessage(`{}`)}, step{"Memory", "read graph"}},
		{ran{Tool: "read", Input: json.RawMessage(`{"file_path":"/job/workspace/visit-notes.txt"}`)}, step{"Read", "visit-notes.txt"}},
		{ran{Tool: "bash", Input: json.RawMessage(`{"command":"bash adder.sh 2 3","description":"x"}`)}, step{"Ran", "bash adder.sh 2 3"}},
		{ran{Tool: "read", Title: "read_file: adder.sh"}, step{"Read", "adder.sh"}},
		{ran{Tool: "execute", Title: "terminal: bash adder.sh 2 3 + 1 command"}, step{"Ran", "bash adder.sh 2 3 + 1 command"}},
	} {
		if got := describe(tc.done); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.done.Tool, got, tc.want)
		}
	}
}
