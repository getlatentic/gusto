package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
)

// kinds are what the protocol calls the things an agent does, by the verb the pages use for each step.
var kinds = map[string]string{
	"Read": "read", "Looked in": "read",
	"Edited": "edit", "Wrote": "edit",
	"Ran":      "execute",
	"Searched": "search", "Searched the web": "fetch", "Fetched": "fetch",
	"Thought": "think", "Planned": "think",
}

func kindOf(what string) string {
	if kind, found := kinds[what]; found {
		return kind
	}
	return "other"
}

// update sends the editor one thing that happened in a session.
func (a *Agent) update(w *wire, session string, update map[string]any) {
	_ = w.notify("session/update", map[string]any{"sessionId": session, "update": update})
}

func (a *Agent) say(w *wire, session, text string) {
	if text == "" {
		return
	}
	a.update(w, session, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}})
}

// prompt is one turn: what the editor said is said in the session's thread, and what the agent does is told back as it goes.
// The turn ends with how it ended. A turn that was stopped, by the editor or by the person elsewhere, ends cancelled.
func (a *Agent) prompt(ctx context.Context, w *wire, got request) {
	var params struct {
		SessionID string            `json:"sessionId"`
		Prompt    []json.RawMessage `json:"prompt"`
	}
	if err := json.Unmarshal(got.Params, &params); err != nil {
		_ = w.fail(got.ID, codeParams, "session/prompt takes a sessionId and a prompt")
		return
	}
	s := a.session(params.SessionID)
	if s == nil {
		_ = w.fail(got.ID, codeParams, "there is no session "+params.SessionID)
		return
	}
	said := textOf(params.Prompt)
	if said == "" {
		_ = w.fail(got.ID, codeParams, "the prompt has no text in it")
		return
	}
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		_ = w.fail(got.ID, codeInvalid, "this session is already working on something: wait, or cancel it")
		return
	}
	s.active, s.cancelled, s.job = true, false, ""
	n, first := s.turns, !s.noted
	s.turns++
	s.noted = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active, s.job = false, ""; s.mu.Unlock() }()

	if first {
		a.say(w, s.id, fmt.Sprintf("I work in %s's folder in the cloud, not in the folder open in your editor, so the files I change are there until the folders are kept in step.\n\n", firstOf(s.instance.Title, s.instance.Harness)))
	}
	sent, err := a.send(ctx, s, said, keyOf(s.id, n, said))
	if err != nil {
		code, message := failure(err)
		_ = w.fail(got.ID, code, message)
		return
	}
	s.mu.Lock()
	s.job = sent.Job
	wasCancelled := s.cancelled
	s.mu.Unlock()
	if wasCancelled {
		_ = a.Cloud.StopJob(ctx, sent.Job)
	}

	ended, err := a.follow(ctx, w, s, sent.Job)
	if err != nil {
		code, message := failure(err)
		_ = w.fail(got.ID, code, message)
		return
	}
	_ = w.answer(got.ID, map[string]string{"stopReason": a.stopReason(w, s, ended)})
}

// send says it, trying again with the same key when the connection fails, so a try that got through is not a second turn.
func (a *Agent) send(ctx context.Context, s *session, said, key string) (helper.Said, error) {
	var last error
	for try := 0; try < 3; try++ {
		sent, err := a.Cloud.Say(ctx, s.instance.ID, s.thread.ID, said, key)
		var refused *helper.Refused
		if err == nil || errors.As(err, &refused) || errors.Is(err, helper.ErrSignedOut) || ctx.Err() != nil {
			return sent, err
		}
		last = err
		a.logf("the send failed (%v), trying again", err)
		select {
		case <-ctx.Done():
			return helper.Said{}, ctx.Err()
		case <-time.After(time.Duration(try+1) * time.Second):
		}
	}
	return helper.Said{}, last
}

// follow tells the editor what the turn does, and gives how it ended. A connection to the server that drops is made again:
// what was told is not told twice.
func (a *Agent) follow(ctx context.Context, w *wire, s *session, job string) (helper.Ended, error) {
	stepsTold, replied := 0, ""
	var last error
	for try := 0; try < 5; try++ {
		seen := 0
		ended, err := a.Cloud.FollowJob(ctx, job, helper.Follower{
			Step: func(step helper.Step) {
				seen++
				if seen <= stepsTold {
					return
				}
				stepsTold = seen
				title := strings.TrimSpace(step.What + " " + step.Detail)
				a.update(w, s.id, map[string]any{
					"sessionUpdate": "tool_call", "toolCallId": fmt.Sprintf("%s-%d", job, seen),
					"title": title, "kind": kindOf(step.What), "status": "completed",
				})
			},
			Reply: func(text string) { replied = a.newText(w, s.id, replied, text) },
		})
		if err == nil {
			replied = a.newText(w, s.id, replied, ended.Reply)
			return ended, nil
		}
		last = err
		if ctx.Err() != nil || errors.Is(err, helper.ErrSignedOut) {
			return helper.Ended{}, err
		}
		var refused *helper.Refused
		if errors.As(err, &refused) {
			return helper.Ended{}, err
		}
		a.logf("following the turn failed (%v), following again", err)
		select {
		case <-ctx.Done():
			return helper.Ended{}, ctx.Err()
		case <-time.After(time.Duration(try+1) * time.Second):
		}
	}
	return helper.Ended{}, last
}

// newText tells the editor what the agent has said since it last did, and gives all that has been said. A reply that does not
// carry on from what was told, as one the server cut short does, is not told: the editor cannot take words back.
func (a *Agent) newText(w *wire, session, told, now string) string {
	if !strings.HasPrefix(now, told) {
		return told
	}
	a.say(w, session, now[len(told):])
	return now
}

// stopReason is how the editor is told the turn ended. A turn that did not finish is said so in words as well, since the
// protocol's reasons have none for a failure.
func (a *Agent) stopReason(w *wire, s *session, ended helper.Ended) string {
	s.mu.Lock()
	asked := s.cancelled
	s.mu.Unlock()
	switch ended.Job.State {
	case "interrupted":
		if asked || ended.Job.StoppedBy != "" {
			return "cancelled"
		}
		a.say(w, s.id, "\n\nThe turn was interrupted before it finished.")
	case "failed":
		why := firstOf(strings.TrimSpace(ended.Stopped), "no reason was given")
		a.say(w, s.id, "\n\nThe turn failed: "+why)
	}
	return "end_turn"
}

// cancel is the editor asking for a turn to stop. It stops the turn in the cloud, which ends it for good: the editor is told
// when the turn's own end arrives, as cancelled.
func (a *Agent) cancel(ctx context.Context, got request) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(got.Params, &params) != nil {
		return
	}
	s := a.session(params.SessionID)
	if s == nil {
		return
	}
	s.mu.Lock()
	s.cancelled = true
	job := s.job
	s.mu.Unlock()
	// A turn that has not been sent yet is stopped as it is sent. One that is sent is stopped now.
	if job != "" {
		if err := a.Cloud.StopJob(ctx, job); err != nil {
			a.logf("the turn could not be stopped: %v", err)
		}
	}
}
