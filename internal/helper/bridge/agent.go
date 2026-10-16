package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/getlatentic/gusto/internal/helper"
)

// Cloud is what the bridge asks of the person's organisation: the calls of the server's API for their own work.
type Cloud interface {
	Instances(ctx context.Context) ([]helper.Instance, error)
	Engines(ctx context.Context) ([]helper.Engine, error)
	StartInstance(ctx context.Context, harness, model string) (helper.Instance, helper.Thread, error)
	NewThread(ctx context.Context, instance, title string) (helper.Thread, error)
	Say(ctx context.Context, instance, thread, said, key string) (helper.Said, error)
	FollowJob(ctx context.Context, job string, told helper.Follower) (helper.Ended, error)
	StopJob(ctx context.Context, job string) error
}

// Agent is the helper as an ACP agent. Its sessions are threads on one of the person's agents in the cloud.
type Agent struct {
	Cloud Cloud
	// Instance names the agent in the cloud to use. Empty is the person's newest, or a new one when they have none.
	Instance string
	// Engine names the kind of agent to start when there is none to use. Empty is the first this server can run.
	Engine  string
	Version string
	// Log gets what is worth a person's eyes. Standard output is the protocol's, so it never goes there.
	Log io.Writer

	mu       sync.Mutex
	sessions map[string]*session
	chosen   *helper.Instance
}

// protocolVersion is the version of the Agent Client Protocol spoken, the one the server speaks to its own harnesses.
const protocolVersion = 1

type session struct {
	id       string
	instance helper.Instance
	thread   helper.Thread

	mu        sync.Mutex
	turns     int
	active    bool
	job       string
	cancelled bool
	noted     bool
}

func (a *Agent) logf(format string, args ...any) {
	if a.Log != nil {
		fmt.Fprintf(a.Log, format+"\n", args...)
	}
}

// Run serves the editor on in and out until it hangs up. A turn the editor leaves running keeps running in the cloud: the
// agent is not here, so closing the editor does not end its work.
func (a *Agent) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	a.sessions = map[string]*session{}
	w := &wire{out: out}
	var running sync.WaitGroup
	err := readRequests(in, func(got request, bad error) {
		if bad != nil {
			_ = w.fail(nil, codeParse, bad.Error())
			return
		}
		running.Add(1)
		go func() {
			defer running.Done()
			a.handle(ctx, w, got)
		}()
	})
	running.Wait()
	return err
}

func (a *Agent) handle(ctx context.Context, w *wire, got request) {
	switch got.Method {
	case "initialize":
		_ = w.answer(got.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"agentCapabilities": map[string]any{
				"loadSession":        false,
				"promptCapabilities": map[string]bool{"image": false, "audio": false, "embeddedContext": false},
				"mcpCapabilities":    map[string]bool{"http": false, "sse": false},
			},
			"agentInfo":   map[string]string{"name": "gusto-helper", "title": "Latentic Agent Cloud", "version": a.Version},
			"authMethods": []any{},
		})
	case "session/new":
		a.newSession(ctx, w, got)
	case "session/prompt":
		a.prompt(ctx, w, got)
	case "session/cancel":
		a.cancel(ctx, got)
	default:
		// A notification the bridge does not know is not answered; a request is told it is not here.
		if got.ID != nil {
			_ = w.fail(got.ID, codeMethod, "the helper does not do "+got.Method)
		}
	}
}

// failure turns an error from the cloud into what the editor should be told.
func failure(err error) (int, string) {
	if errors.Is(err, helper.ErrSignedOut) || errors.Is(err, helper.ErrNotSignedIn) {
		return codeAuth, "this computer is not signed in to your organisation: run `gusto-helper login`"
	}
	var refused *helper.Refused
	if errors.As(err, &refused) {
		return codeInternal, refused.Message
	}
	return codeInternal, err.Error()
}

func (a *Agent) newSession(ctx context.Context, w *wire, got request) {
	var params struct {
		Cwd string `json:"cwd"`
	}
	if err := json.Unmarshal(got.Params, &params); err != nil {
		_ = w.fail(got.ID, codeParams, "session/new takes a cwd")
		return
	}
	instance, err := a.agentToUse(ctx)
	if err != nil {
		code, message := failure(err)
		_ = w.fail(got.ID, code, message)
		return
	}
	thread, err := a.Cloud.NewThread(ctx, instance.ID, "Editor session")
	if err != nil {
		code, message := failure(err)
		_ = w.fail(got.ID, code, message)
		return
	}
	s := &session{id: instance.ID + "/" + thread.ID, instance: instance, thread: thread}
	a.mu.Lock()
	a.sessions[s.id] = s
	a.mu.Unlock()
	a.logf("session %s on %s (%s)", s.id, instance.Title, instance.Harness)
	_ = w.answer(got.ID, map[string]string{"sessionId": s.id})
}

// agentToUse is the agent in the cloud the editor's sessions go to: the one named, else the person's newest, else a new one.
// It is chosen once, so the sessions of one editor are on one agent.
func (a *Agent) agentToUse(ctx context.Context) (helper.Instance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.chosen != nil {
		return *a.chosen, nil
	}
	all, err := a.Cloud.Instances(ctx)
	if err != nil {
		return helper.Instance{}, err
	}
	var picked *helper.Instance
	for i := range all {
		switch {
		case a.Instance != "" && all[i].ID == a.Instance:
			picked = &all[i]
		case a.Instance == "" && (picked == nil || all[i].Started > picked.Started):
			picked = &all[i]
		}
	}
	if a.Instance != "" && picked == nil {
		return helper.Instance{}, fmt.Errorf("you have no agent called %s: gusto-helper agents lists yours", a.Instance)
	}
	if picked == nil {
		engine, err := a.engine(ctx)
		if err != nil {
			return helper.Instance{}, err
		}
		started, _, err := a.Cloud.StartInstance(ctx, engine, "")
		if err != nil {
			return helper.Instance{}, err
		}
		picked = &started
	}
	a.chosen = picked
	return *picked, nil
}

// engine is the kind of agent to start: the one named, or the first this server can run.
func (a *Agent) engine(ctx context.Context) (string, error) {
	if a.Engine != "" {
		return a.Engine, nil
	}
	engines, err := a.Cloud.Engines(ctx)
	if err != nil {
		return "", err
	}
	for _, engine := range engines {
		if engine.Ready && len(engine.Models) > 0 {
			return engine.Name, nil
		}
	}
	return "", errors.New("your organisation's server cannot start an agent for you yet")
}

func (a *Agent) session(id string) *session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[id]
}

// textOf is what the editor said, from the content blocks of a prompt: the text, and the name of anything it linked.
func textOf(blocks []json.RawMessage) string {
	var parts []string
	for _, raw := range blocks {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
			URI  string `json:"uri"`
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &block) != nil {
			continue
		}
		switch block.Type {
		case "text":
			parts = append(parts, block.Text)
		case "resource_link":
			parts = append(parts, "[the editor pointed at "+firstOf(block.Name, block.URI)+"]")
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// keyOf is the request id of a send: the same session, the same place in it and the same words are the same send, so a send
// that was tried again is one turn.
func keyOf(session string, n int, said string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s", session, n, said)))
	return "acp-" + hex.EncodeToString(sum[:12])
}
