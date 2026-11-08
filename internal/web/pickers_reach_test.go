package web

import (
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/wire"
)

// reachLikeTheDemo is the demo's credentials: anthropic answers only Anthropic's shape, deepseek answers OpenAI's and
// translates Anthropic's, gemma answers chat and translates the rest.
func reachLikeTheDemo(model string, shape wire.Shape) (bool, bool) {
	switch {
	case strings.HasPrefix(model, "anthropic/"):
		return shape == wire.Messages, shape == wire.Messages
	case strings.HasPrefix(model, "deepseek/"):
		return shape != wire.Messages, true
	}
	return shape == wire.Chat, true
}

func TestAnAgentIsOfferedEachModelOnceAndOnlyThroughACredentialItCanUse(t *testing.T) {
	s := &Server{reaches: reachLikeTheDemo}
	offered := []string{"gemma/google.gemma-4-26b-a4b", "anthropic/deepseek-flash", "deepseek/deepseek-flash"}
	for shape, want := range map[wire.Shape][]string{
		wire.Chat:      {"gemma/google.gemma-4-26b-a4b", "deepseek/deepseek-flash"},
		wire.Responses: {"gemma/google.gemma-4-26b-a4b", "deepseek/deepseek-flash"},
		wire.Messages:  {"gemma/google.gemma-4-26b-a4b", "anthropic/deepseek-flash"},
	} {
		if got := s.usableBy(shape, offered); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", shape, got, want)
		}
	}
}

func TestWithNothingToAskEveryModelIsOffered(t *testing.T) {
	offered := []string{"a/x", "b/x"}
	if got := (&Server{}).usableBy(wire.Chat, offered); !slices.Equal(got, offered) {
		t.Errorf("%v", got)
	}
}
