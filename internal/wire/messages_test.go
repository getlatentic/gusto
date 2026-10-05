package wire

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// sent is a translated call as a provider reads it.
type sent struct {
	Model    string `json:"model"`
	Messages []struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
	Tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	ToolChoice    any  `json:"tool_choice"`
	MaxTokens     *int `json:"max_tokens"`
	Stream        bool `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

func translated(t *testing.T, from Shape, fixture string, options Options) (sent, map[string]any) {
	t.Helper()
	body, err := os.ReadFile("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	call, err := Translate(from, body, options)
	if err != nil {
		t.Fatal(err)
	}
	var out sent
	if err := json.Unmarshal(call.Body, &out); err != nil {
		t.Fatal(err)
	}
	loose := map[string]any{}
	_ = json.Unmarshal(call.Body, &loose)
	return out, loose
}

func textIn(raw json.RawMessage) string {
	var text string
	_ = json.Unmarshal(raw, &text)
	return text
}

// rolesAlternate checks the order every Chat Completions template takes: one system message, if any, first; then a
// user's message; and a tool's answer only right after the call it answers.
func rolesAlternate(t *testing.T, out sent) {
	t.Helper()
	open := map[string]bool{}
	for i, message := range out.Messages {
		switch message.Role {
		case "system":
			if i != 0 {
				t.Errorf("message %d is a system message after the start", i)
			}
		case "tool":
			if !open[message.ToolCallID] {
				t.Errorf("message %d answers %q, which no call before it made", i, message.ToolCallID)
			}
			delete(open, message.ToolCallID)
		default:
			if len(open) > 0 {
				t.Errorf("message %d comes before every call is answered: %v", i, open)
			}
			if i > 0 && message.Role == "user" && out.Messages[i-1].Role == "user" {
				t.Errorf("messages %d and %d are both a user's", i-1, i)
			}
		}
		for _, call := range message.ToolCalls {
			open[call.ID] = true
		}
	}
}

func TestClaudeCodesFirstCallBecomesAChatCallWithItsToolsAndNoneOfAnthropicsOwnFields(t *testing.T) {
	out, loose := translated(t, Messages, "claude-code-first.json", Options{})
	if out.Model != "test-model" || !out.Stream || out.StreamOptions == nil || !out.StreamOptions.IncludeUsage {
		t.Errorf("model %q stream %v options %+v", out.Model, out.Stream, out.StreamOptions)
	}
	if out.MaxTokens == nil || *out.MaxTokens != 32000 {
		t.Errorf("max tokens %v", out.MaxTokens)
	}
	for _, field := range []string{"thinking", "system", "metadata", "context_management", "output_config"} {
		if _, found := loose[field]; found {
			t.Errorf("%s was sent on", field)
		}
	}
	if out.Messages[0].Role != "system" || !strings.Contains(textIn(out.Messages[0].Content), "(system 0)") || !strings.Contains(textIn(out.Messages[0].Content), "(system 2)") {
		t.Errorf("the system prompt is not the first message, whole: %s", out.Messages[0].Content)
	}
	rolesAlternate(t, out)
	names := []string{}
	for _, tool := range out.Tools {
		names = append(names, tool.Function.Name)
		if tool.Type != "function" || !strings.Contains(string(tool.Function.Parameters), `"properties"`) {
			t.Errorf("tool %s: %s %s", tool.Function.Name, tool.Type, tool.Function.Parameters)
		}
	}
	if strings.Join(names, ",") != "Bash,Edit,Read" {
		t.Errorf("tools %v", names)
	}
}

func TestATurnWithAToolsResultBecomesTheCallAndItsAnswerInOrder(t *testing.T) {
	out, _ := translated(t, Messages, "claude-code-after-tool.json", Options{})
	rolesAlternate(t, out)
	var call, answer bool
	for _, message := range out.Messages {
		for _, made := range message.ToolCalls {
			if made.ID == "toolu_1" && made.Function.Name == "Bash" && strings.Contains(made.Function.Arguments, `"echo hi"`) && message.Role == "assistant" {
				call = true
			}
		}
		if message.Role == "tool" && message.ToolCallID == "toolu_1" && textIn(message.Content) == "hi" {
			answer = true
		}
	}
	if !call || !answer {
		t.Errorf("call %v answer %v: %+v", call, answer, out.Messages)
	}
	last := out.Messages[len(out.Messages)-1]
	if last.Role != "user" || !strings.Contains(textIn(last.Content), "(system text)") {
		t.Errorf("a system message in the middle of the conversation is not a user's message at its end: %+v", last)
	}
}

func TestTheMostAHarnessAsksForIsHeldToWhatTheProviderAllows(t *testing.T) {
	out, _ := translated(t, Messages, "claude-code-first.json", Options{MaxOutput: 8192})
	if out.MaxTokens == nil || *out.MaxTokens != 8192 {
		t.Errorf("%v", out.MaxTokens)
	}
}

func TestAnUnansweredCallIsAnsweredWithANoteAndAProvidersOwnToolIsLeftOut(t *testing.T) {
	body := `{"model":"m","max_tokens":10,"tools":[{"name":"Read","input_schema":{"type":"object"}},{"type":"web_search_20250305","name":"web_search"}],
		"tool_choice":{"type":"any","disable_parallel_tool_use":true},
		"messages":[{"role":"user","content":"look"},{"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"s"},{"type":"tool_use","id":"t1","name":"Read","input":{}}]},
		{"role":"user","content":[{"type":"text","text":"never mind"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}]}`
	call, err := Translate(Messages, []byte(body), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out sent
	_ = json.Unmarshal(call.Body, &out)
	rolesAlternate(t, out)
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "Read" {
		t.Errorf("tools %+v", out.Tools)
	}
	if out.ToolChoice != "required" || !strings.Contains(string(call.Body), `"parallel_tool_calls":false`) {
		t.Errorf("choice %v: %s", out.ToolChoice, call.Body)
	}
	if strings.Contains(string(call.Body), "thinking") || !strings.Contains(string(call.Body), "data:image/png;base64,AAAA") || !strings.Contains(string(call.Body), "(no result)") {
		t.Errorf("%s", call.Body)
	}
}
