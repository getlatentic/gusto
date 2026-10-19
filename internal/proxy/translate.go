package proxy

import (
	"io"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/wire"
)

// translation is what goes out for a call: the body and path it came with when its credential's provider answers its
// shape, and the call made in Chat Completions, with its path, when the provider has to have it translated. The Call
// is empty when nothing is translated.
func translation(credential Credential, rest string, body []byte) ([]byte, string, wire.Call, error) {
	from := wire.Of(rest)
	if from == "" || !credential.Translates(from) {
		return body, rest, wire.Call{}, nil
	}
	call, err := wire.Translate(from, body, wire.Options{MaxOutput: credential.MaxOutput})
	if err != nil {
		return nil, "", wire.Call{From: from}, err
	}
	return call.Body, wire.ChatPath(rest), call, nil
}

// asChat takes off the headers that belong to the shape a call was made in. A provider that answers Chat Completions
// has no use for Anthropic's version and beta headers, and some refuse a beta they do not know.
func asChat(header http.Header) {
	for name := range header {
		if strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			header.Del(name)
		}
	}
	header.Set("Content-Type", "application/json")
}

// countedReader is a reader that knows how much has been read from it, for the record of a translated call, which is what
// the provider sent rather than what the agent was given.
type countedReader struct {
	from io.Reader
	n    int64
}

func (c *countedReader) Read(p []byte) (int, error) {
	n, err := c.from.Read(p)
	c.n += int64(n)
	return n, err
}
