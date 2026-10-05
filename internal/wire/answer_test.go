package wire

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatStream is a streamed Chat Completions answer: text in two pieces, then two tool calls whose arguments arrive
// interleaved, then why it stopped, then its usage, as a provider asked to include usage sends it.
const chatStream = `data: {"model":"g","choices":[{"delta":{"role":"assistant","content":""}}]}

data: {"model":"g","choices":[{"delta":{"content":"Hel"}}]}

data: {"model":"g","choices":[{"delta":{"content":"lo"}}]}

data: {"model":"g","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"Bash","arguments":""}}]}}]}

data: {"model":"g","choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"Read","arguments":"{\"path\":"}}]}}]}

data: {"model":"g","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"ls\"}"}}]}}]}

data: {"model":"g","choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"a.go\"}"}}]}}]}

data: {"model":"g","choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: {"model":"g","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":40}}}

data: [DONE]

`

type event struct {
	kind string
	data map[string]any
}

func eventsIn(t *testing.T, body string) []event {
	t.Helper()
	var out []event
	var kind string
	lines := bufio.NewScanner(strings.NewReader(body))
	for lines.Scan() {
		line := lines.Text()
		if name, found := strings.CutPrefix(line, "event: "); found {
			kind = name
		}
		if data, found := strings.CutPrefix(line, "data: "); found {
			parsed := map[string]any{}
			if err := json.Unmarshal([]byte(data), &parsed); err != nil {
				t.Fatalf("%s: %v", data, err)
			}
			out = append(out, event{kind, parsed})
		}
	}
	return out
}

func answered(t *testing.T, call Call, status int, contentType, upstream string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	_ = call.Answer(recorder, status, contentType, strings.NewReader(upstream))
	return recorder
}

func TestAStreamedChatAnswerIsAMessagesStreamClaudeCodeCanFollow(t *testing.T) {
	got := answered(t, Call{From: Messages, streamed: true, model: "g"}, 200, "text/event-stream", chatStream)
	if got.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("%v", got.Header())
	}
	kinds := []string{}
	text, blocks := "", map[float64]map[string]any{}
	var stop map[string]any
	for _, each := range eventsIn(t, got.Body.String()) {
		kinds = append(kinds, each.kind)
		if each.data["type"] != each.kind {
			t.Errorf("event %s says it is %v", each.kind, each.data["type"])
		}
		switch each.kind {
		case "content_block_start":
			blocks[each.data["index"].(float64)] = each.data["content_block"].(map[string]any)
		case "content_block_delta":
			delta := each.data["delta"].(map[string]any)
			block := blocks[each.data["index"].(float64)]
			if delta["type"] == "text_delta" {
				text += delta["text"].(string)
			} else {
				block["partial"] = delta["partial_json"]
			}
		case "message_delta":
			stop = each.data
		}
	}
	want := "message_start content_block_start content_block_delta content_block_delta content_block_stop " +
		"content_block_start content_block_delta content_block_stop content_block_start content_block_delta content_block_stop message_delta message_stop"
	if strings.Join(kinds, " ") != want {
		t.Errorf("events:\n%s\nwant\n%s", strings.Join(kinds, " "), want)
	}
	if text != "Hello" {
		t.Errorf("text %q", text)
	}
	if blocks[1]["id"] != "call_a" || blocks[1]["name"] != "Bash" || blocks[1]["partial"] != `{"command":"ls"}` ||
		blocks[2]["id"] != "call_b" || blocks[2]["partial"] != `{"path":"a.go"}` {
		t.Errorf("tool blocks %v", blocks)
	}
	usage := stop["usage"].(map[string]any)
	if stop["delta"].(map[string]any)["stop_reason"] != "tool_use" || usage["input_tokens"] != 60.0 || usage["cache_read_input_tokens"] != 40.0 || usage["output_tokens"] != 20.0 {
		t.Errorf("%v", stop)
	}
}

func TestAStreamedChatAnswerIsAResponsesStreamCodexCanFollow(t *testing.T) {
	got := answered(t, Call{From: Responses, streamed: true, model: "g"}, 200, "text/event-stream", chatStream)
	all := eventsIn(t, got.Body.String())
	kinds := []string{}
	for i, each := range all {
		kinds = append(kinds, each.kind)
		if each.data["sequence_number"] != float64(i) {
			t.Errorf("event %d is numbered %v", i, each.data["sequence_number"])
		}
	}
	want := "response.created response.in_progress response.output_item.added response.content_part.added " +
		"response.output_text.delta response.output_text.delta response.output_text.done response.content_part.done response.output_item.done " +
		"response.output_item.added response.function_call_arguments.delta response.function_call_arguments.done response.output_item.done " +
		"response.output_item.added response.function_call_arguments.delta response.function_call_arguments.done response.output_item.done response.completed"
	if strings.Join(kinds, " ") != want {
		t.Errorf("events:\n%s\nwant\n%s", strings.Join(kinds, " "), want)
	}
	done := all[len(all)-1].data["response"].(map[string]any)
	output := done["output"].([]any)
	if len(output) != 3 || done["status"] != "completed" {
		t.Fatalf("%v", done)
	}
	message := output[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	first, second := output[1].(map[string]any), output[2].(map[string]any)
	if message["text"] != "Hello" || first["call_id"] != "call_a" || first["arguments"] != `{"command":"ls"}` || second["name"] != "Read" || second["arguments"] != `{"path":"a.go"}` {
		t.Errorf("%v", output)
	}
	usage := done["usage"].(map[string]any)
	if usage["input_tokens"] != 100.0 || usage["output_tokens"] != 20.0 || usage["input_tokens_details"].(map[string]any)["cached_tokens"] != 40.0 {
		t.Errorf("%v", usage)
	}
}

const chatWhole = `{"model":"g","choices":[{"message":{"role":"assistant","content":"Hi","tool_calls":[{"id":"call_a","type":"function","function":{"name":"apply_patch","arguments":"{\"input\":\"*** Begin Patch\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`

func TestAWholeAnswerIsOneMessageOrOneResponse(t *testing.T) {
	got := answered(t, Call{From: Messages, model: "g"}, 200, "application/json", chatWhole)
	var message map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &message); err != nil || got.Code != 200 {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	content := message["content"].([]any)
	if message["type"] != "message" || message["stop_reason"] != "tool_use" || len(content) != 2 || content[0].(map[string]any)["text"] != "Hi" {
		t.Errorf("%v", message)
	}
	got = answered(t, Call{From: Responses, model: "g", freeText: map[string]bool{"apply_patch": true}}, 200, "application/json", chatWhole)
	var response map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	call := response["output"].([]any)[1].(map[string]any)
	if response["object"] != "response" || call["type"] != "custom_tool_call" || call["input"] != "*** Begin Patch" || call["call_id"] != "call_a" {
		t.Errorf("%v", response)
	}
}

func TestAStreamAskedForIsAStreamEvenWhenTheProviderAnsweredWhole(t *testing.T) {
	got := answered(t, Call{From: Messages, streamed: true, model: "g"}, 200, "application/json", chatWhole)
	kinds := []string{}
	for _, each := range eventsIn(t, got.Body.String()) {
		kinds = append(kinds, each.kind)
	}
	if len(kinds) < 3 || kinds[0] != "message_start" || kinds[len(kinds)-1] != "message_stop" {
		t.Errorf("%v", kinds)
	}
}

func TestAProvidersRefusalIsAnErrorInTheHarnesssShapeWithItsStatus(t *testing.T) {
	refusal := `{"error":{"message":"max_tokens is too large","type":"invalid_request_error"}}`
	got := answered(t, Call{From: Messages, streamed: true}, 400, "application/json", refusal)
	var said struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &said); err != nil || got.Code != 400 || said.Type != "error" ||
		said.Error.Type != "invalid_request_error" || said.Error.Message != "max_tokens is too large" {
		t.Errorf("%d %s", got.Code, got.Body)
	}
	got = answered(t, Call{From: Responses, streamed: true}, 503, "text/plain", "busy")
	if got.Code != 503 || !strings.Contains(got.Body.String(), `"message":"busy"`) || !strings.Contains(got.Body.String(), `"server_error"`) {
		t.Errorf("%d %s", got.Code, got.Body)
	}
}

func TestAStreamCutOffEndsInAnErrorAndNotInAFinishedAnswer(t *testing.T) {
	cut := strings.Join(strings.Split(chatStream, "\n\n")[:4], "\n\n") + "\n\n"
	for _, from := range []Shape{Messages, Responses} {
		recorder := httptest.NewRecorder()
		err := Call{From: from, streamed: true, model: "g"}.Answer(recorder, 200, "text/event-stream", strings.NewReader(cut))
		all := eventsIn(t, recorder.Body.String())
		last := all[len(all)-1]
		if err == nil || last.kind != "error" || strings.Contains(recorder.Body.String(), "message_stop") || strings.Contains(recorder.Body.String(), "response.completed") {
			t.Errorf("%s: %v, last %v", from, err, last)
		}
	}
}

func TestAWholeAnswerThatCannotBeReadIsAnErrorAndNotAnEmptyMessage(t *testing.T) {
	got := answered(t, Call{From: Messages, model: "g"}, 200, "application/json", "not json")
	if got.Code != http.StatusBadGateway || !strings.Contains(got.Body.String(), `"type":"error"`) {
		t.Errorf("%d %s", got.Code, got.Body)
	}
}

func TestEachShapeIsKnownByItsPathAndGoesToChatCompletionsAtTheSamePrefix(t *testing.T) {
	for path, want := range map[string]Shape{"/v1/messages": Messages, "/v1/responses": Responses, "/v1/chat/completions": Chat, "/v1/models": "", "/v1/messages/count_tokens": ""} {
		if Of(path) != want {
			t.Errorf("%s is %q", path, Of(path))
		}
	}
	if ChatPath("/v1/messages") != "/v1/chat/completions" || ChatPath("/v1/responses") != "/v1/chat/completions" || ChatPath("/responses") != "/chat/completions" {
		t.Errorf("%s %s", ChatPath("/v1/messages"), ChatPath("/v1/responses"))
	}
}
