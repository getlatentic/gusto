package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// question is a request from a container, read whole before it leaves so the
// ledger can see which model it names and a cap can refuse it before anything
// is spent.
type question struct {
	body  []byte
	model string
}

// askLimit is the largest request the proxy reads. A long conversation sent
// whole is several megabytes; one this size is not a request to a model.
const askLimit = 32 << 20

var errTooLarge = errors.New("that request is larger than a call to a model")

func readQuestion(r *http.Request) (question, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, askLimit+1))
	if err != nil {
		return question{}, err
	}
	if len(body) > askLimit {
		return question{}, errTooLarge
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(body, &fields) != nil {
		// Not a JSON object, such as a GET with no body: it names no model
		// and goes as it came.
		return question{body: body}, nil
	}
	var model string
	_ = json.Unmarshal(fields["model"], &model)
	return question{body: countable(fields, body), model: model}, nil
}

// countable asks a streamed answer to carry its usage. An OpenAI-compatible
// stream reports usage only when stream_options.include_usage is set, and a
// agent that does not set it would otherwise spend without a count. The
// provider then sends one more event with the usage and no choices, which the
// documented stream format allows for.
//
// Every value is carried over as the bytes it arrived as; only the order of
// the top-level fields can change.
func countable(fields map[string]json.RawMessage, body []byte) []byte {
	var streamed bool
	if json.Unmarshal(fields["stream"], &streamed) != nil || !streamed {
		return body
	}
	options := map[string]json.RawMessage{}
	_ = json.Unmarshal(fields["stream_options"], &options)
	options["include_usage"] = json.RawMessage("true")
	joined, err := json.Marshal(options)
	if err != nil {
		return body
	}
	fields["stream_options"] = joined
	rewritten, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return rewritten
}
