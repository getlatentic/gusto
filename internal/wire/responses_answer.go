package wire

import (
	"encoding/json"
	"sort"
	"time"
)

// responsesAnswer writes an answer as the Responses API gives one: as numbered events when the call was streamed, and
// as one response at the end when it was not. Text is written as it arrives, and a tool call once its arguments are,
// like messagesAnswer. A tool Codex calls with free text is written as such a call, with its text taken out of the
// one argument it was given here.
type responsesAnswer struct {
	out      *events
	streamed bool
	freeText map[string]bool
	id       string
	model    string
	created  int64
	sequence int
	output   []map[string]any
	message  map[string]any
	written  string
	calls    map[int]*pendingCall
}

func newResponsesAnswer(out *events, streamed bool, freeText map[string]bool) *responsesAnswer {
	return &responsesAnswer{out: out, streamed: streamed, freeText: freeText, id: newID("resp_"),
		created: time.Now().Unix(), calls: map[int]*pendingCall{}}
}

func (r *responsesAnswer) send(kind string, data map[string]any) error {
	if !r.streamed {
		return nil
	}
	data["type"], data["sequence_number"] = kind, r.sequence
	r.sequence++
	return r.out.send(kind, data)
}

func (r *responsesAnswer) response(status string) map[string]any {
	output := r.output
	if output == nil {
		output = []map[string]any{}
	}
	return map[string]any{"id": r.id, "object": "response", "created_at": r.created, "status": status,
		"model": r.model, "output": output}
}

func (r *responsesAnswer) begin(model string) error {
	r.model = model
	if err := r.send("response.created", map[string]any{"response": r.response("in_progress")}); err != nil {
		return err
	}
	return r.send("response.in_progress", map[string]any{"response": r.response("in_progress")})
}

func (r *responsesAnswer) text(delta string) error {
	if err := r.flushCalls(); err != nil {
		return err
	}
	if r.message == nil {
		r.written = ""
		r.message = map[string]any{"type": "message", "id": newID("msg_"), "role": "assistant", "status": "in_progress", "content": []any{}}
		at := len(r.output)
		if err := r.send("response.output_item.added", map[string]any{"output_index": at, "item": r.message}); err != nil {
			return err
		}
		if err := r.send("response.content_part.added", map[string]any{"item_id": r.message["id"], "output_index": at, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}}); err != nil {
			return err
		}
	}
	r.written += delta
	return r.send("response.output_text.delta", map[string]any{"item_id": r.message["id"], "output_index": len(r.output), "content_index": 0, "delta": delta})
}

func (r *responsesAnswer) closeMessage() error {
	if r.message == nil {
		return nil
	}
	at, id := len(r.output), r.message["id"]
	done := map[string]any{"type": "output_text", "text": r.written, "annotations": []any{}}
	finished := map[string]any{"type": "message", "id": id, "role": "assistant", "status": "completed", "content": []any{done}}
	r.message = nil
	r.output = append(r.output, finished)
	for _, event := range []struct {
		kind string
		data map[string]any
	}{
		{"response.output_text.done", map[string]any{"item_id": id, "output_index": at, "content_index": 0, "text": r.written}},
		{"response.content_part.done", map[string]any{"item_id": id, "output_index": at, "content_index": 0, "part": done}},
		{"response.output_item.done", map[string]any{"output_index": at, "item": finished}},
	} {
		if err := r.send(event.kind, event.data); err != nil {
			return err
		}
	}
	return nil
}

func (r *responsesAnswer) tool(index int, id, name string) error {
	if err := r.closeMessage(); err != nil {
		return err
	}
	r.calls[index] = &pendingCall{id: id, name: name}
	return nil
}

func (r *responsesAnswer) arguments(index int, delta string) error {
	if call, found := r.calls[index]; found {
		call.arguments += delta
	}
	return nil
}

func (r *responsesAnswer) flushCalls() error {
	indexes := make([]int, 0, len(r.calls))
	for index := range r.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		if err := r.writeCall(r.calls[index]); err != nil {
			return err
		}
	}
	r.calls = map[int]*pendingCall{}
	return nil
}

func (r *responsesAnswer) writeCall(call *pendingCall) error {
	at := len(r.output)
	arguments := call.arguments
	if !json.Valid([]byte(arguments)) {
		arguments = "{}"
	}
	if r.freeText[call.name] {
		var wrapped struct {
			Input string `json:"input"`
		}
		_ = json.Unmarshal([]byte(arguments), &wrapped)
		item := map[string]any{"type": "custom_tool_call", "id": newID("ctc_"), "call_id": call.id, "name": call.name, "input": wrapped.Input, "status": "completed"}
		r.output = append(r.output, item)
		if err := r.send("response.output_item.added", map[string]any{"output_index": at, "item": item}); err != nil {
			return err
		}
		return r.send("response.output_item.done", map[string]any{"output_index": at, "item": item})
	}
	id := newID("fc_")
	started := map[string]any{"type": "function_call", "id": id, "call_id": call.id, "name": call.name, "arguments": "", "status": "in_progress"}
	finished := map[string]any{"type": "function_call", "id": id, "call_id": call.id, "name": call.name, "arguments": arguments, "status": "completed"}
	r.output = append(r.output, finished)
	for _, event := range []struct {
		kind string
		data map[string]any
	}{
		{"response.output_item.added", map[string]any{"output_index": at, "item": started}},
		{"response.function_call_arguments.delta", map[string]any{"item_id": id, "output_index": at, "delta": arguments}},
		{"response.function_call_arguments.done", map[string]any{"item_id": id, "output_index": at, "arguments": arguments}},
		{"response.output_item.done", map[string]any{"output_index": at, "item": finished}},
	} {
		if err := r.send(event.kind, event.data); err != nil {
			return err
		}
	}
	return nil
}

func (r *responsesAnswer) end(stop Stop, usage Usage) error {
	if err := r.closeMessage(); err != nil {
		return err
	}
	if err := r.flushCalls(); err != nil {
		return err
	}
	status, kind := "completed", "response.completed"
	done := r.response(status)
	if stop == StopLength || stop == StopFilter {
		status, kind = "incomplete", "response.incomplete"
		done = r.response(status)
		reason := "max_output_tokens"
		if stop == StopFilter {
			reason = "content_filter"
		}
		done["incomplete_details"] = map[string]string{"reason": reason}
	}
	done["usage"] = map[string]any{"input_tokens": usage.Input, "output_tokens": usage.Output, "total_tokens": usage.Input + usage.Output,
		"input_tokens_details": map[string]int64{"cached_tokens": usage.Cached}, "output_tokens_details": map[string]int64{"reasoning_tokens": 0}}
	if !r.streamed {
		return r.out.whole(done)
	}
	return r.send(kind, map[string]any{"response": done})
}

// responsesError is an error in the body the Responses API gives one in.
func responsesError(status int, message string) map[string]any {
	kind := "invalid_request_error"
	if status >= 500 {
		kind = "server_error"
	}
	return map[string]any{"error": map[string]any{"message": message, "type": kind, "param": nil, "code": nil}}
}
