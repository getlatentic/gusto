package main

import (
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/pkg/job"
)

// A turn in a conversation works on the workspace of the instance it is a turn on. In a
// container that is mounted over the task's. A sandbox on a gateway has its own files,
// so the instance's workspace is copied into the task's before the turn and back after
// it. It is copied back only when the turn got as far as writing its record, which is
// after its workspace came out of the sandbox, so a turn that failed on the way leaves
// the instance as it was.

// beforeTurn puts the instance's workspace where the task's is.
func beforeTurn(instances *instance.Store, spec job.Spec, dir job.Dir) error {
	if spec.Session == "" {
		return nil
	}
	return replaceTree(instances.Workspace(spec.Session), dir.Workspace())
}

// afterTurn brings the workspace back to the instance, and learns what the agent calls
// the conversation, so the next turn is handed it and does not start again.
func afterTurn(instances *instance.Store, spec job.Spec, dir job.Dir) error {
	if spec.Session == "" {
		return nil
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return nil
	}
	if err := replaceTree(dir.Workspace(), instances.Workspace(spec.Session)); err != nil {
		return err
	}
	learnConversation(instances, spec, manifest.Session)
	return nil
}

// learnConversation keeps what the agent calls a conversation, in the thread it belongs to.
func learnConversation(instances *instance.Store, spec job.Spec, remembered string) {
	if spec.Session != "" && spec.Thread != "" {
		_ = instances.Said(spec.Session, spec.Thread, "", remembered)
	}
}
