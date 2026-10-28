package main

import (
	"os"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/job"
)

// A turn in a conversation works on the workspace of the instance it is a turn on. In a
// container that is mounted over the task's. A sandbox on a gateway has its own files,
// so the instance's workspace is copied into the task's before the turn and back after
// it. It is copied back only when the turn got as far as writing its record, which is
// after its workspace came out of the sandbox, so a turn that failed on the way leaves
// the instance as it was.
//
// A room's agents run at the same time, so a turn of one works on a copy of the room's folder
// that roomRuns makes and merges, and none of this applies to its workspace.

// staying is what a turn needs that outlives its container: the conversation's
// working directory, and the memory the agent keeps of the conversation.
// An agent stores that memory in its own data directory rather than beside the
// files it edits, which is why there are two. A room's agent works on a copy of
// the room's folder that is in its task directory already, so only the memory is mounted.
func staying(instances *instance.Store, spec job.Spec) []sandbox.Mount {
	if spec.Session == "" {
		return nil
	}
	memory := sandbox.Mount{From: stateDir(instances, spec), To: "/root/.local/share"}
	if isRoomTurn(spec) {
		return []sandbox.Mount{memory}
	}
	return []sandbox.Mount{{From: instances.Workspace(spec.Session), To: "/job/workspace"}, memory}
}

// ownerOf is whose a task's container is: this server's organization, the person who pays for the task as the same opaque id
// their computer has, and the agent's instance.
func ownerOf(index *jobs.Index, dir job.Dir, spec job.Spec) sandbox.Owner {
	owner := sandbox.Owner{Org: os.Getenv("GUSTO_ORG"), Instance: spec.Session}
	if person := personWhoPays(index, dir); person != "" {
		owner.Person = computer.ID(owner.Org, person)
	}
	return owner
}

// beforeTurn puts the instance's workspace where the task's is.
func beforeTurn(instances *instance.Store, spec job.Spec, dir job.Dir) error {
	if spec.Session == "" || isRoomTurn(spec) {
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
	if !isRoomTurn(spec) {
		if err := replaceTree(dir.Workspace(), instances.Workspace(spec.Session)); err != nil {
			return err
		}
	}
	learnConversation(instances, spec, manifest.Session)
	return nil
}

// learnConversation keeps what the agent calls a conversation, in the thread it belongs to.
func learnConversation(instances *instance.Store, spec job.Spec, remembered string) {
	if spec.Session != "" && spec.Thread != "" {
		_ = instances.Learned(spec.Session, spec.Thread, spec.Agent, remembered)
	}
}
