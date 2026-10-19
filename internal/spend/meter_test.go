package spend

import (
	"strings"
	"testing"
)

// These two are the documented OpenAI-compatible shape. The measured ones
// further down are DeepSeek's own answers on each wire.
const (
	plainAnswer = `{"id":"x","model":"m","choices":[{"message":{"content":"yes"}}],` +
		`"usage":{"prompt_tokens":1200,"completion_tokens":300,"total_tokens":1500}}`
	streamedAnswer = "data: {\"choices\":[{\"delta\":{\"content\":\"y\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"es\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":2,\"total_tokens\":42}}\n\n" +
		"data: [DONE]\n\n"
)

func metered(contentType, answer string, piece int) *Usage {
	meter := NewMeter(contentType)
	for start := 0; start < len(answer); start += piece {
		end := min(start+piece, len(answer))
		meter.Write([]byte(answer[start:end]))
	}
	return meter.Usage()
}

func TestAPlainAnswerIsCountedFromItsUsage(t *testing.T) {
	usage := metered("application/json", plainAnswer, 7)
	if usage == nil || usage.Input != 1200 || usage.Output != 300 {
		t.Fatalf("usage %+v", usage)
	}
}

// A stream arrives in pieces that do not respect its lines, so the meter has
// to read across them.
func TestAStreamedAnswerIsCountedFromItsLastEvent(t *testing.T) {
	for _, piece := range []int{1, 5, 64, len(streamedAnswer)} {
		usage := metered("text/event-stream; charset=utf-8", streamedAnswer, piece)
		if usage == nil || usage.Input != 40 || usage.Output != 2 {
			t.Fatalf("in pieces of %d, usage %+v", piece, usage)
		}
	}
}

func TestAnAnswerWithoutUsageIsUnmeasuredRatherThanFree(t *testing.T) {
	for _, tc := range []struct{ contentType, answer string }{
		{"application/json", `{"choices":[{"message":{"content":"yes"}}]}`},
		{"application/json", `{"usage":{"total_tokens":10}}`},
		{"application/json", `not json`},
		{"text/event-stream", "data: {\"choices\":[{\"delta\":{}}]}\n\ndata: [DONE]\n\n"},
	} {
		if usage := metered(tc.contentType, tc.answer, 3); usage != nil {
			t.Fatalf("%q read as %+v", tc.answer, usage)
		}
	}
}

func TestAnAnswerTooLargeToHoldIsUnmeasured(t *testing.T) {
	huge := `{"pad":"` + strings.Repeat("x", meterLimit) + `","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	if usage := metered("application/json", huge, 1<<20); usage != nil {
		t.Fatalf("an answer past the limit read as %+v", usage)
	}
}

// Two providers, two namings, one pair of numbers. Both bodies here are real:
// they came back from the same credential, one from /v1/chat/completions and
// one from /v1/responses, and the second counted nothing before this.
func TestUsageIsCountedWhicheverWayTheProviderNamesIt(t *testing.T) {
	for _, each := range []struct {
		api  string
		body string
	}{
		{"chat completions", `{"usage":{"completion_tokens":16,"prompt_tokens":69,"total_tokens":85}}`},
		{"responses", `{"usage":{"input_tokens":69,"input_tokens_details":{"cached_tokens":0},"output_tokens":16,"output_tokens_details":{"reasoning_tokens":14},"total_tokens":85}}`},
		{"anthropic messages", `{"usage":{"input_tokens":69,"output_tokens":16}}`},
	} {
		t.Run(each.api, func(t *testing.T) {
			meter := NewMeter("application/json")
			if _, err := meter.Write([]byte(each.body)); err != nil {
				t.Fatal(err)
			}
			usage := meter.Usage()
			if usage == nil {
				t.Fatal("the call counted nothing, which reads as free")
			}
			if usage.Input != 69 || usage.Output != 16 {
				t.Errorf("counted %d in and %d out", usage.Input, usage.Output)
			}
		})
	}
}

// A naming nobody knows counts nothing rather than guessing a number.
func TestUsageNobodyRecognizesIsNotInvented(t *testing.T) {
	meter := NewMeter("application/json")
	if _, err := meter.Write([]byte(`{"usage":{"tokens_used":85}}`)); err != nil {
		t.Fatal(err)
	}
	if usage := meter.Usage(); usage != nil {
		t.Errorf("a usage nobody knows was read as %+v", usage)
	}
}

// DeepSeek's answers on 2026-09-27, cut to their usage. A chat completion
// counts the cache inside prompt_tokens; Anthropic's shape counts it beside
// input_tokens, and its stream says the input first and the output last.
const (
	deepseekChat = `{"usage":{"prompt_tokens":1034,"completion_tokens":2,"total_tokens":1036,` +
		`"prompt_tokens_details":{"cached_tokens":896},"prompt_cache_hit_tokens":896,"prompt_cache_miss_tokens":138}}`
	deepseekResponses = "data: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":33," +
		"\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":16,\"total_tokens\":49}}}\n\n"
	anthropicStream = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":" +
		"{\"input_tokens\":34,\"cache_creation_input_tokens\":10,\"cache_read_input_tokens\":500,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":40}}\n\n"
)

func TestTheCachedShareOfAChatCompletionIsCounted(t *testing.T) {
	usage := metered("application/json", deepseekChat, 9)
	if usage == nil || *usage != (Usage{Input: 1034, Cached: 896, Output: 2}) {
		t.Fatalf("usage %+v", usage)
	}
}

func TestAStreamedResponsesAnswerIsCountedFromItsCompletedEvent(t *testing.T) {
	usage := metered("text/event-stream", deepseekResponses, 11)
	if usage == nil || *usage != (Usage{Input: 33, Output: 16}) {
		t.Fatalf("usage %+v", usage)
	}
}

// Anthropic's stream says the input in message_start and the output again in
// message_delta, which may carry nothing else. Keeping only the last event would
// lose the input, and keeping only the first would count one output token.
func TestAnAnthropicStreamIsCountedAcrossItsEvents(t *testing.T) {
	usage := metered("text/event-stream", anthropicStream, 13)
	if usage == nil || *usage != (Usage{Input: 544, Cached: 500, CacheWrite: 10, Output: 40}) {
		t.Fatalf("usage %+v", usage)
	}
}
