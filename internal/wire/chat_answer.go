package wire

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type chatUsage struct {
	Prompt     int64 `json:"prompt_tokens"`
	Completion int64 `json:"completion_tokens"`
	Details    *struct {
		Cached int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u *chatUsage) usage() Usage {
	if u == nil {
		return Usage{}
	}
	counted := Usage{Input: u.Prompt, Output: u.Completion}
	if u.Details != nil {
		counted.Cached = u.Details.Cached
	}
	return counted
}

type chatToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// chatChunk is one event of a streamed answer.
type chatChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content   *string         `json:"content"`
			ToolCalls []chatToolDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

// chatAnswer is an answer given whole.
type chatAnswer struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   *string         `json:"content"`
			ToolCalls []chatToolDelta `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

// stopOf is why a Chat Completions model stopped, in the words every shape is told.
func stopOf(reason string) Stop {
	switch reason {
	case "length":
		return StopLength
	case "tool_calls", "function_call":
		return StopTools
	case "content_filter":
		return StopFilter
	}
	return StopEnd
}

// toolsSeen tells a sink each tool call once, however many events name it, and gives one a provider left unnamed an
// identifier, since the harness answers a call by it.
type toolsSeen map[int]bool

func (seen toolsSeen) tell(to sink, delta chatToolDelta) error {
	if !seen[delta.Index] {
		seen[delta.Index] = true
		id := delta.ID
		if id == "" {
			id = newID("call_")
		}
		if err := to.tool(delta.Index, id, delta.Function.Name); err != nil {
			return err
		}
	}
	if delta.Function.Arguments == "" {
		return nil
	}
	return to.arguments(delta.Index, delta.Function.Arguments)
}

// readChatStream tells a sink a streamed Chat Completions answer as it arrives. A stream that ends without saying
// why, as one cut off does, is an error, so that a harness is not told a broken answer was a finished one.
func readChatStream(body io.Reader, model string, to sink) error {
	if err := to.begin(model); err != nil {
		return err
	}
	lines := bufio.NewScanner(body)
	lines.Buffer(make([]byte, 64<<10), 16<<20)
	seen := toolsSeen{}
	var stop Stop
	var usage Usage
	for lines.Scan() {
		data, found := strings.CutPrefix(lines.Text(), "data:")
		data = strings.TrimSpace(data)
		if !found || data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = chunk.Usage.usage()
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				if err := to.text(*choice.Delta.Content); err != nil {
					return err
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				if err := seen.tell(to, delta); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				stop = stopOf(*choice.FinishReason)
			}
		}
	}
	if err := lines.Err(); err != nil {
		return err
	}
	if stop == "" {
		return errors.New("the model's answer ended before it said it was finished")
	}
	return to.end(stop, usage)
}

// readChatWhole tells a sink an answer given whole, as if it had been streamed in one piece.
func readChatWhole(body io.Reader, model string, to sink) error {
	var answer chatAnswer
	if err := json.NewDecoder(body).Decode(&answer); err != nil {
		return err
	}
	if len(answer.Choices) == 0 {
		return errors.New("the model's answer has no choices in it")
	}
	if err := to.begin(model); err != nil {
		return err
	}
	choice := answer.Choices[0]
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		if err := to.text(*choice.Message.Content); err != nil {
			return err
		}
	}
	seen := toolsSeen{}
	for i, call := range choice.Message.ToolCalls {
		call.Index = i
		if err := seen.tell(to, call); err != nil {
			return err
		}
	}
	return to.end(stopOf(choice.FinishReason), answer.Usage.usage())
}
