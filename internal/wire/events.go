package wire

import (
	"encoding/json"
	"io"
	"net/http"
)

// events writes an answer to the harness: server-sent events, each handed on as soon as it is written, since a harness
// that streams shows what the model says as it says it, or one body at the end.
type events struct {
	to io.Writer
}

func (e *events) send(kind string, data any) error {
	text, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(e.to, "event: "+kind+"\ndata: "+string(text)+"\n\n"); err != nil {
		return err
	}
	if flusher, ok := e.to.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (e *events) whole(body any) error {
	return json.NewEncoder(e.to).Encode(body)
}
