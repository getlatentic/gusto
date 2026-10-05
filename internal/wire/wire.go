// Package wire translates between the shapes of API that coding agents call a model in, so that a harness can use a
// model whose provider answers in another shape.
//
// Claude Code calls Anthropic's Messages API, Codex the Responses API, and most other harnesses Chat Completions,
// while many providers answer Chat Completions alone. A call made in Messages or Responses is translated into Chat
// Completions on the way out, and the answer, streamed or whole, is translated back. What one shape has and the
// other cannot carry, such as Anthropic's thinking blocks or a hosted search tool, is left out rather than imitated.
package wire

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Shape is the API a call is made in.
type Shape string

const (
	Chat      Shape = "chat"
	Messages  Shape = "messages"
	Responses Shape = "responses"
)

// Of is the shape a request to that path is made in, or "" for a path that is not a call to a model, such as a list
// of models, which goes as it came.
func Of(path string) Shape {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return Chat
	case strings.HasSuffix(path, "/messages"):
		return Messages
	case strings.HasSuffix(path, "/responses"):
		return Responses
	}
	return ""
}

// ChatPath is the path a call made in one shape is sent to once it is in Chat Completions: the same prefix, with the
// shape's own ending replaced.
func ChatPath(path string) string {
	for _, ending := range []string{"/messages", "/responses"} {
		if trimmed, found := strings.CutSuffix(path, ending); found {
			return trimmed + "/chat/completions"
		}
	}
	return path
}

// Stop is why a model stopped.
type Stop string

const (
	// StopEnd is a model that finished what it had to say.
	StopEnd Stop = "end"
	// StopLength is a model cut off by the most it may write.
	StopLength Stop = "length"
	// StopTools is a model that stopped to have its tools called.
	StopTools Stop = "tools"
	// StopFilter is an answer the provider withheld.
	StopFilter Stop = "filter"
)

// Usage is a provider's count of one call. Input includes Cached, the part of it read from a cache.
type Usage struct {
	Input, Cached, Output int64
}

// sink is what an answer is told as it is read, whatever shape it arrived in: its text as it comes, each tool call it
// makes and the arguments of each, and how it ended. A tool call's arguments may arrive in pieces, and the pieces of
// two calls may interleave.
type sink interface {
	begin(model string) error
	text(delta string) error
	tool(index int, id, name string) error
	arguments(index int, delta string) error
	end(stop Stop, usage Usage) error
}

// newID is an identifier the shape's own provider would have made, with its own prefix.
func newID(prefix string) string {
	random := make([]byte, 12)
	_, _ = rand.Read(random)
	return prefix + hex.EncodeToString(random)
}
