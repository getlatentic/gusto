package wire

import (
	"strings"
	"testing"
)

func TestCodexsFirstCallBecomesAChatCallWithItsFunctionsAndNotWhatOnlyOpenAIRuns(t *testing.T) {
	out, loose := translated(t, Responses, "codex-first.json", Options{})
	if out.Model != "test-model" || !out.Stream {
		t.Errorf("model %q stream %v", out.Model, out.Stream)
	}
	for _, field := range []string{"instructions", "input", "reasoning", "include", "store", "prompt_cache_key", "client_metadata"} {
		if _, found := loose[field]; found {
			t.Errorf("%s was sent on", field)
		}
	}
	rolesAlternate(t, out)
	first := textIn(out.Messages[0].Content)
	if out.Messages[0].Role != "system" || !strings.Contains(first, "(instructions)") || !strings.Contains(first, "(developer text)") {
		t.Errorf("the instructions and the developer's message are not the first message: %q", first)
	}
	names := []string{}
	for _, tool := range out.Tools {
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "exec_command,view_image" {
		t.Errorf("a namespace or a hosted tool was sent on, or a function was not: %v", names)
	}
	if out.ToolChoice != "auto" {
		t.Errorf("%v", out.ToolChoice)
	}
}

func TestCodexsCallAndItsOutputBecomeAnAssistantsCallAndAToolsAnswer(t *testing.T) {
	out, _ := translated(t, Responses, "codex-after-tool.json", Options{})
	rolesAlternate(t, out)
	n := len(out.Messages)
	call, answer := out.Messages[n-2], out.Messages[n-1]
	if call.Role != "assistant" || len(call.ToolCalls) != 1 || call.ToolCalls[0].ID != "call_1" || call.ToolCalls[0].Function.Name != "exec_command" || call.ToolCalls[0].Function.Arguments != `{"cmd": "echo hi"}` {
		t.Errorf("%+v", call)
	}
	if answer.Role != "tool" || answer.ToolCallID != "call_1" || textIn(answer.Content) != "(command output)" {
		t.Errorf("%+v", answer)
	}
}

func TestAToolCodexCallsWithFreeTextTakesItAsOneArgumentAndComesBackAsFreeText(t *testing.T) {
	body := `{"model":"m","stream":false,"input":[{"role":"user","content":"patch it"},
		{"type":"custom_tool_call","call_id":"c1","name":"apply_patch","input":"*** Begin Patch"},
		{"type":"custom_tool_call_output","call_id":"c1","output":"done"}],
		"tools":[{"type":"custom","name":"apply_patch","description":"d","format":{"type":"grammar"}}]}`
	call, err := Translate(Responses, []byte(body), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !call.freeText["apply_patch"] || !strings.Contains(string(call.Body), `{\"input\":\"*** Begin Patch\"}`) || !strings.Contains(string(call.Body), `"required":["input"]`) {
		t.Errorf("%v %s", call.freeText, call.Body)
	}
}
