package spend

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Meter reads the provider's usage out of an answer as it is relayed, without
// holding it up. It is written to alongside the container, so the agent sees
// every byte at the moment it would have without a proxy.
//
// It reads chat completions, the responses API and Anthropic's messages, plain
// and streamed, in the shapes DeepSeek answered them with on 2026-09-27.
type Meter struct {
	streamed bool
	// pending is the part of a streamed answer after its last newline, and the
	// whole of a plain one.
	pending []byte
	spilled bool
	said    *reported
	// text is how many bytes of generated text a streamed answer has carried, and final says the
	// provider has given its closing count of output tokens.
	text  int64
	final bool
}

// meterLimit is how much of an answer the meter holds to find its usage. An
// answer larger than this is relayed whole and left unmeasured, rather than
// held in memory without bound.
const meterLimit = 16 << 20

// NewMeter reads an answer of this content type.
func NewMeter(contentType string) *Meter {
	return &Meter{streamed: strings.HasPrefix(contentType, "text/event-stream")}
}

func (m *Meter) Write(given []byte) (int, error) {
	if m.spilled {
		return len(given), nil
	}
	m.pending = append(m.pending, given...)
	if m.streamed {
		m.events()
	}
	if len(m.pending) > meterLimit {
		m.spilled, m.pending = true, nil
	}
	return len(given), nil
}

// events reads every complete line of a stream, and keeps the rest for the
// next write. A stream can say its usage over several events, Anthropic's
// input at the start and its output at the end, so each field keeps the last
// value any event gave it.
func (m *Meter) events() {
	for {
		end := bytes.IndexByte(m.pending, '\n')
		if end < 0 {
			return
		}
		line := bytes.TrimSpace(m.pending[:end])
		m.pending = m.pending[end+1:]
		if data, found := bytes.CutPrefix(line, []byte("data:")); found {
			m.read(bytes.TrimSpace(data))
		}
	}
}

// Usage is what the provider said the call cost in tokens, or nil when the
// answer did not say. A naming this does not know counts nothing, which is
// reported as unmeasured rather than as free.
func (m *Meter) Usage() *Usage {
	if !m.streamed && !m.spilled {
		m.read(m.pending)
	}
	if m.said == nil {
		return nil
	}
	return m.said.usage()
}

// Running is what the call has used so far, for a cap that has to act before the answer ends.
// Providers give their count of output tokens once, at the end, so until then the output is
// estimated from the text seen at three bytes a token, which leans toward stopping early.
func (m *Meter) Running() Usage {
	var counted Usage
	if m.said != nil {
		counted = m.said.partial()
	}
	if !m.final {
		counted.Output = (m.text + 2) / 3
	}
	return counted
}

// observe counts the text one event carries and notes a closing count of output tokens.
func (m *Meter) observe(text []byte) {
	var event struct {
		Usage *struct {
			Output     *int64 `json:"output_tokens"`
			Completion *int64 `json:"completion_tokens"`
		} `json:"usage"`
		Delta *struct {
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
			Partial  string `json:"partial_json"`
		} `json:"delta"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal(text, &event) != nil {
		return
	}
	if event.Usage != nil && (event.Usage.Output != nil || event.Usage.Completion != nil) {
		m.final = true
	}
	if event.Delta != nil {
		m.text += int64(len(event.Delta.Text) + len(event.Delta.Thinking) + len(event.Delta.Partial))
	}
	for _, choice := range event.Choices {
		m.text += int64(len(choice.Delta.Content))
	}
}

func (m *Meter) read(text []byte) {
	if m.streamed {
		m.observe(text)
	}
	found := usageIn(text)
	if found == nil {
		return
	}
	if m.said == nil {
		m.said = &reported{}
	}
	m.said.merge(found)
}
