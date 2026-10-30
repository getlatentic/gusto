package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits bound one model call, so that one runaway generation cannot hold a turn: a model that writes far more than it was
// asked for, or a stream that stops sending without ending. Zero is no limit.
type Limits struct {
	// MaxOutput is the most output tokens a call may ask for. A call that asks for more is asked for this, and a call that
	// names no limit is given it.
	MaxOutput int
	// Silence is how long a call may go without the model sending anything before it is ended.
	Silence time.Duration
}

// UseLimits bounds the model calls made from now on.
func (p *Proxy) UseLimits(limits Limits) { p.limits.Store(&limits) }

func (p *Proxy) currentLimits() Limits {
	if held := p.limits.Load(); held != nil {
		return *held
	}
	return Limits{}
}

// outputFields are the names the wires give the most output tokens: chat completions has had two, messages one, and responses one.
var outputFields = []string{"max_tokens", "max_completion_tokens", "max_output_tokens"}

// capOutput asks for at most most output tokens in a call to a model at path. A limit the call names above it is lowered, one
// below it is kept, and a call that names none is given one in the field its wire reads. A body that is not a call to a
// model goes as it came.
func capOutput(body []byte, path string, most int) []byte {
	if most <= 0 {
		return body
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(body, &fields) != nil || fields["model"] == nil {
		return body
	}
	named := false
	for _, name := range outputFields {
		raw, found := fields[name]
		if !found {
			continue
		}
		named = true
		var asked int
		if json.Unmarshal(raw, &asked) == nil && asked > 0 && asked <= most {
			continue
		}
		fields[name] = json.RawMessage(strconv.Itoa(most))
	}
	if !named {
		name := "max_tokens"
		if strings.HasSuffix(path, "/responses") {
			name = "max_output_tokens"
		}
		fields[name] = json.RawMessage(strconv.Itoa(most))
	}
	capped, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return capped
}

// errSilent is why a call was ended: the model sent nothing for as long as a call may wait.
var errSilent = errors.New("the model sent nothing for too long, so the call was ended")

// silence ends a call that has gone quiet. It is started when the wait for the model begins, and every read of the model's
// answer that brings something starts the wait again.
type silence struct {
	after time.Duration
	end   context.CancelCauseFunc
	mu    sync.Mutex
	timer *time.Timer
}

func newSilence(after time.Duration, end context.CancelCauseFunc) *silence {
	return &silence{after: after, end: end}
}

// wait starts, or starts again, the time the model has to send something.
func (s *silence) wait() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer == nil {
		s.timer = time.AfterFunc(s.after, func() { s.end(errSilent) })
		return
	}
	s.timer.Reset(s.after)
}

// stop is the end of the call: nothing is waited for any more.
func (s *silence) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
}

// reading is the model's answer, each piece of which starts the wait again.
func (s *silence) reading(from io.Reader) io.Reader {
	if s == nil {
		return from
	}
	return heard{from: from, s: s}
}

type heard struct {
	from io.Reader
	s    *silence
}

func (h heard) Read(p []byte) (int, error) {
	n, err := h.from.Read(p)
	if n > 0 {
		h.s.wait()
	}
	return n, err
}
