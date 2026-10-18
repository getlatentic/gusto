package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/redact"
)

// UseRedaction sets what is masked in what passes through the proxy: a request before it leaves for the provider, and the answer
// before it reaches the agent. Nil masks nothing. It can be set again while the proxy runs, and a call in flight finishes with
// what it began with.
func (p *Proxy) UseRedaction(redactor *redact.Redactor) { p.redactor.Store(redactor) }

// maskRequest masks a request's body, whether it is JSON or plain text.
func maskRequest(redactor *redact.Redactor, body []byte) ([]byte, redact.Counts) {
	if json.Valid(body) {
		return redactor.JSON(body)
	}
	return redactor.Text(body)
}

// maskedAnswer is a writer that masks what is written through it, event by event for a stream and whole for a body, and passes
// anything that is not text as it is. What it masked is counted.
type maskedAnswer struct {
	to       http.ResponseWriter
	redactor *redact.Redactor
	mode     answerMode
	pending  []byte
	counts   redact.Counts
	started  bool
	failed   error
}

type answerMode int

const (
	passing answerMode = iota
	streaming
	whole
)

// mostBuffered is the largest answer that is held to be masked whole. An answer from a model is far smaller.
const mostBuffered = 32 << 20

var errAnswerTooLarge = errors.New("the answer is larger than the proxy masks")

func newMaskedAnswer(to http.ResponseWriter, redactor *redact.Redactor) *maskedAnswer {
	return &maskedAnswer{to: to, redactor: redactor, counts: redact.Counts{}}
}

func (m *maskedAnswer) Header() http.Header { return m.to.Header() }

func (m *maskedAnswer) WriteHeader(status int) {
	if m.started {
		return
	}
	m.started = true
	kind := strings.ToLower(m.to.Header().Get("Content-Type"))
	switch {
	case m.to.Header().Get("Content-Encoding") != "":
		// What is packed cannot be read here, and the proxy asks providers not to pack it.
	case strings.HasPrefix(kind, "text/event-stream"):
		m.mode = streaming
	case strings.HasPrefix(kind, "application/json"), strings.HasPrefix(kind, "text/"), strings.Contains(kind, "+json"):
		m.mode = whole
		m.to.Header().Del("Content-Length")
	}
	m.to.WriteHeader(status)
}

func (m *maskedAnswer) Write(given []byte) (int, error) {
	if !m.started {
		m.WriteHeader(http.StatusOK)
	}
	if m.failed != nil {
		return 0, m.failed
	}
	switch m.mode {
	case passing:
		return m.to.Write(given)
	case whole:
		if len(m.pending)+len(given) > mostBuffered {
			m.failed = errAnswerTooLarge
			return 0, m.failed
		}
		m.pending = append(m.pending, given...)
		return len(given), nil
	}
	m.pending = append(m.pending, given...)
	for {
		end, size := endOfEvent(m.pending)
		if end < 0 {
			break
		}
		if err := m.send(m.pending[:end+size]); err != nil {
			return 0, err
		}
		m.pending = m.pending[end+size:]
	}
	return len(given), nil
}

// endOfEvent is where the first whole event in a stream ends: at a blank line, written as two line feeds or as two of the pairs of
// a carriage return and a line feed.
func endOfEvent(stream []byte) (end, size int) {
	lf, crlf := bytes.Index(stream, []byte("\n\n")), bytes.Index(stream, []byte("\r\n\r\n"))
	switch {
	case lf >= 0 && (crlf < 0 || lf < crlf):
		return lf, 2
	case crlf >= 0:
		return crlf, 4
	}
	return -1, 0
}

func (m *maskedAnswer) send(event []byte) error {
	masked, found := m.redactor.Event(event)
	m.counts.Add(found)
	if _, err := m.to.Write(masked); err != nil {
		return err
	}
	m.Flush()
	return nil
}

// Flush sends what has been masked so far. A body that is held whole waits to be masked.
func (m *maskedAnswer) Flush() {
	if m.mode == whole {
		return
	}
	if flusher, ok := m.to.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Close masks and sends what is left: the last event of a stream that did not end with a blank line, or a body held whole.
func (m *maskedAnswer) Close() error {
	if m.failed != nil || len(m.pending) == 0 {
		return m.failed
	}
	rest := m.pending
	m.pending = nil
	if m.mode == streaming {
		return m.send(rest)
	}
	masked, found := maskRequest(m.redactor, rest)
	m.counts.Add(found)
	_, err := m.to.Write(masked)
	return err
}

// Unwrap lets a handler reach the connection's own controls.
func (m *maskedAnswer) Unwrap() http.ResponseWriter { return m.to }
