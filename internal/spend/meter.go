package spend

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Meter reads the provider's usage out of an answer as it is relayed, without
// holding it up. It is written to alongside the container, so the harness sees
// every byte at the moment it would have without a proxy.
//
// The shape it reads is the documented OpenAI-compatible one: a `usage` object
// with `prompt_tokens` and `completion_tokens`, at the top of a plain answer, or
// in the last `data:` event of a streamed one. That path is written against the
// documentation and has not yet been measured against a real upstream.
type Meter struct {
	streamed bool
	// pending is the part of a streamed answer after its last newline, and the
	// whole of a plain one.
	pending []byte
	spilled bool
	usage   *Usage
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
// next write. The last usage a stream carries is the one kept.
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
// answer did not say.
func (m *Meter) Usage() *Usage {
	if !m.streamed && !m.spilled {
		m.read(m.pending)
	}
	return m.usage
}

func (m *Meter) read(text []byte) {
	var answer struct {
		Usage *struct {
			Prompt     *int64 `json:"prompt_tokens"`
			Completion *int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(text, &answer) != nil || answer.Usage == nil {
		return
	}
	if answer.Usage.Prompt == nil || answer.Usage.Completion == nil {
		return
	}
	m.usage = &Usage{Input: *answer.Usage.Prompt, Output: *answer.Usage.Completion}
}
