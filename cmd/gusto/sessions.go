package main

import (
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/pkg/task"
)

// learnConversation keeps what the agent calls a conversation, in the thread it belongs to, so the next turn is handed it and
// does not start again.
func learnConversation(instances *instance.Store, spec task.Spec, remembered string) {
	if spec.Session != "" && spec.Thread != "" {
		_ = instances.Learned(spec.Session, spec.Thread, spec.Agent, remembered)
	}
}
