package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Call is a call a harness made in its own shape, as the Chat Completions call it is sent as, and what it takes to give
// the answer back in the harness's shape.
type Call struct {
	From Shape
	// Body is the Chat Completions call.
	Body     []byte
	streamed bool
	model    string
	freeText map[string]bool
}

// Translate makes a call in Messages or Responses into one in Chat Completions.
func Translate(from Shape, body []byte, options Options) (Call, error) {
	var (
		chat     chatRequest
		freeText map[string]bool
		err      error
	)
	switch from {
	case Messages:
		chat, err = fromMessages(body, options)
	case Responses:
		chat, freeText, err = fromResponses(body, options)
	default:
		return Call{}, fmt.Errorf("a call in %q is not translated", from)
	}
	if err != nil {
		return Call{}, err
	}
	translated, err := json.Marshal(chat)
	if err != nil {
		return Call{}, err
	}
	return Call{From: from, Body: translated, streamed: chat.Stream, model: chat.Model, freeText: freeText}, nil
}

// Fail tells the harness that the call could not be made, in its own shape, so that it shows the reason rather than
// a body it cannot read.
func Fail(w http.ResponseWriter, from Shape, status int, message string) {
	body := responsesError(status, message)
	if from == Messages {
		body = messagesError(status, message)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Answer gives the provider's answer to the harness in the shape it called in: streamed when it asked for a stream,
// whatever the provider did, and whole when it did not. A provider's refusal goes back as an error in the harness's
// shape with the provider's status. An error once a stream has begun is told as the shape's own error event, since
// the status is already sent; it is also returned, for the record.
func (c Call) Answer(w http.ResponseWriter, status int, contentType string, body io.Reader) error {
	if status < 200 || status > 299 {
		said, _ := io.ReadAll(io.LimitReader(body, 64<<10))
		message := reasonIn(said)
		Fail(w, c.From, status, message)
		return fmt.Errorf("the model answered %d: %s", status, message)
	}
	read := readChatWhole
	if strings.HasPrefix(contentType, "text/event-stream") {
		read = readChatStream
	}
	if !c.streamed {
		var held bytes.Buffer
		if err := read(body, c.model, c.sink(&events{to: &held})); err != nil {
			Fail(w, c.From, http.StatusBadGateway, "the model's answer could not be read: "+err.Error())
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(held.Bytes())
		return err
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	out := &events{to: w}
	if err := read(body, c.model, c.sink(out)); err != nil {
		c.failStream(out, err)
		return err
	}
	return nil
}

func (c Call) sink(out *events) sink {
	if c.From == Messages {
		return newMessagesAnswer(out, c.streamed)
	}
	return newResponsesAnswer(out, c.streamed, c.freeText)
}

func (c Call) failStream(out *events, cause error) {
	message := "the model's answer could not be read: " + cause.Error()
	if c.From == Messages {
		_ = out.send("error", messagesError(http.StatusBadGateway, message))
		return
	}
	_ = out.send("error", map[string]any{"type": "error", "code": "server_error", "message": message, "param": nil})
}

// reasonIn is the message in a provider's error, wherever it keeps it, or the body itself.
func reasonIn(body []byte) string {
	var said struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Detail  string          `json:"detail"`
	}
	if json.Unmarshal(body, &said) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var plain string
		switch {
		case json.Unmarshal(said.Error, &nested) == nil && nested.Message != "":
			return nested.Message
		case json.Unmarshal(said.Error, &plain) == nil && plain != "":
			return plain
		case said.Message != "":
			return said.Message
		case said.Detail != "":
			return said.Detail
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "the model gave no reason"
	}
	return text
}
