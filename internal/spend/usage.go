package spend

import "encoding/json"

// Usage is the provider's own count of one call. Input is every input token,
// Cached and CacheWrite included: those two are the parts of it billed at a
// different rate.
type Usage struct {
	Input      int64
	Cached     int64
	CacheWrite int64
	Output     int64
}

// reported is the usage fields an answer, or one event of a stream, carries.
// Each is a pointer because a stream says them across several events, and a
// field an event leaves out is not a field it says is zero.
type reported struct {
	Prompt     *int64 `json:"prompt_tokens"`
	Completion *int64 `json:"completion_tokens"`
	Input      *int64 `json:"input_tokens"`
	Output     *int64 `json:"output_tokens"`
	// OpenAI's two wires put the cached share of the input in a detail.
	PromptDetails *struct {
		Cached *int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	InputDetails *struct {
		Cached *int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	// Anthropic's leaves cache reads and writes out of input_tokens and names
	// them beside it.
	CacheRead    *int64 `json:"cache_read_input_tokens"`
	CacheCreated *int64 `json:"cache_creation_input_tokens"`
}

// usageIn finds the usage in one answer or event, wherever the wire keeps it:
// at the top of a chat completion and of an Anthropic message_delta, under
// response in the responses API's completed event, and under message in
// Anthropic's message_start.
func usageIn(text []byte) *reported {
	var answer struct {
		Usage    *reported `json:"usage"`
		Response *struct {
			Usage *reported `json:"usage"`
		} `json:"response"`
		Message *struct {
			Usage *reported `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(text, &answer) != nil {
		return nil
	}
	switch {
	case answer.Usage != nil:
		return answer.Usage
	case answer.Response != nil && answer.Response.Usage != nil:
		return answer.Response.Usage
	case answer.Message != nil && answer.Message.Usage != nil:
		return answer.Message.Usage
	}
	return nil
}

// merge keeps the latest value of every field said so far.
func (r *reported) merge(later *reported) {
	for _, pair := range []struct{ into, from **int64 }{
		{&r.Prompt, &later.Prompt}, {&r.Completion, &later.Completion},
		{&r.Input, &later.Input}, {&r.Output, &later.Output},
		{&r.CacheRead, &later.CacheRead}, {&r.CacheCreated, &later.CacheCreated},
	} {
		if *pair.from != nil {
			*pair.into = *pair.from
		}
	}
	if later.PromptDetails != nil {
		r.PromptDetails = later.PromptDetails
	}
	if later.InputDetails != nil {
		r.InputDetails = later.InputDetails
	}
}

// usage is what was said, as one count, or nil when the input or the output
// was never said: half a count billed as a whole one would read as cheaper
// than it was.
func (r *reported) usage() *Usage {
	in, out := r.Prompt, r.Completion
	if in == nil && out == nil {
		in, out = r.Input, r.Output
	}
	if in == nil || out == nil {
		return nil
	}
	counted := &Usage{Input: *in, Output: *out}
	if r.CacheRead != nil || r.CacheCreated != nil {
		counted.Cached, counted.CacheWrite = deref(r.CacheRead), deref(r.CacheCreated)
		counted.Input += counted.Cached + counted.CacheWrite
		return counted
	}
	if r.PromptDetails != nil {
		counted.Cached = deref(r.PromptDetails.Cached)
	} else if r.InputDetails != nil {
		counted.Cached = deref(r.InputDetails.Cached)
	}
	return counted
}

func deref(count *int64) int64 {
	if count == nil {
		return 0
	}
	return *count
}
