package proxy

import (
	"context"
	"testing"

	"github.com/getlatentic/gusto/pkg/wire"
)

func TestAModelIsReachedOnlyThroughACredentialThatAnswersOrTranslatesTheShape(t *testing.T) {
	p := New(NewTokens(), Credentials{
		"anthropic": {Address: "https://api.example/anthropic", Auth: Anthropic},
		"deepseek":  {Address: "https://api.example", Auth: Bearer},
		"gemma":     {Address: "https://api.example/openai", Auth: Bearer, Wires: []string{"chat"}},
	}, func(string) (string, bool) { return "", false }, nil, Allowlist{})
	for _, each := range []struct {
		model      string
		shape      wire.Shape
		native, ok bool
	}{
		{"anthropic/deepseek-flash", wire.Messages, true, true},
		{"anthropic/deepseek-flash", wire.Chat, false, false},
		{"deepseek/deepseek-flash", wire.Messages, false, true},
		{"deepseek/deepseek-flash", wire.Chat, true, true},
		{"gemma/gemma-4", wire.Responses, false, true},
		{"nobody/model", wire.Chat, false, false},
	} {
		native, ok := p.Reaches(context.Background(), each.model, each.shape)
		if native != each.native || ok != each.ok {
			t.Errorf("%s in %s: native %v ok %v, want %v %v", each.model, each.shape, native, ok, each.native, each.ok)
		}
	}
}
