// Package mirror keeps a folder on a person's computer the same as an agent's folder in the cloud, in both directions, as
// real files on both sides. It decides what to do from three pictures: what each side has now, and what both had when they
// last agreed. That third picture is what tells a change from a deletion, and a change on one side from a change on both.
package mirror

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Entry is a file as one side has it. Its content is what it is: two files with the same Hash are the same file, whatever
// their times say.
type Entry struct {
	Hash string
	Size int64
}

// Snapshot is the files of one side, by their path from the folder's root, written with forward slashes. A path that is not
// in it is a file that is not there.
type Snapshot map[string]Entry

// Kind is what to do about one path.
type Kind string

const (
	// Pull makes the local file what the cloud's is: new, changed, or put back where only the local file was deleted.
	Pull Kind = "pull"
	// Push makes the cloud's file what the local one is.
	Push Kind = "push"
	// DeleteLocal removes the local file, because the cloud's was deleted and the local one was not changed since.
	DeleteLocal Kind = "delete-local"
	// DeleteRemote removes the cloud's file, because the local one was deleted and the cloud's was not changed since.
	DeleteRemote Kind = "delete-remote"
	// Conflict is a path both sides changed, in different ways. Nothing is lost: see Conflicted.
	Conflict Kind = "conflict"
	// Settle is a path both sides already agree on, which only the record of the last agreement needs to learn.
	Settle Kind = "settle"
	// Forget is a path neither side has any more.
	Forget Kind = "forget"
)

// Action is one thing to do about one path.
type Action struct {
	Kind Kind
	Path string
	// Copy is, for a Conflict, the path the cloud's version is kept at beside the local file.
	Copy string
}

// Plan is what to do so the two sides agree, in the order of the paths, which makes it the same for the same pictures.
//
// The rule, in one place:
//
//   - A change on one side only goes to the other. A file that is new, changed or deleted on one side while the other did not
//     touch it is made so on the other side.
//   - A change beats a deletion. If one side deleted a file and the other changed it, the changed file stays, on both sides.
//     Nothing that somebody edited is removed because somebody else removed the file.
//   - When both sides changed a file, or both made a new file of the same name with different content, neither version is lost
//     and neither is merged. The local file stays where it is, as it is, and keeps the name. The cloud's version is saved
//     beside it as <name>.conflict-<time>.<ext>. Both then go to the cloud, so the agent has them too, and the next time the
//     two sides are compared they agree.
//   - Which side wins the name is the local one: a person saving a file by hand while an agent edits the same one is the
//     person's act, and what the agent wrote is one rename away.
//
// now names the conflict copies, and taken says which paths already exist on either side, so a copy never overwrites a file.
func Plan(base, local, remote Snapshot, now time.Time, taken func(path string) bool) []Action {
	paths := map[string]struct{}{}
	for _, side := range []Snapshot{base, local, remote} {
		for name := range side {
			paths[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)

	var actions []Action
	used := map[string]bool{}
	for _, name := range names {
		l, here := local[name]
		r, there := remote[name]
		b, was := base[name]
		var act Action
		switch {
		case !here && !there:
			act = Action{Kind: Forget, Path: name}
		case here && there && l.Hash == r.Hash:
			act = Action{Kind: Settle, Path: name}
		case here && !there:
			switch {
			case !was:
				act = Action{Kind: Push, Path: name}
			case l.Hash == b.Hash:
				act = Action{Kind: DeleteLocal, Path: name}
			default:
				act = Action{Kind: Push, Path: name}
			}
		case !here && there:
			switch {
			case !was:
				act = Action{Kind: Pull, Path: name}
			case r.Hash == b.Hash:
				act = Action{Kind: DeleteRemote, Path: name}
			default:
				act = Action{Kind: Pull, Path: name}
			}
		case was && l.Hash == b.Hash:
			act = Action{Kind: Pull, Path: name}
		case was && r.Hash == b.Hash:
			act = Action{Kind: Push, Path: name}
		default:
			act = Action{Kind: Conflict, Path: name, Copy: conflictCopy(name, now, func(candidate string) bool {
				_, onLocal := local[candidate]
				_, onRemote := remote[candidate]
				return onLocal || onRemote || used[candidate] || (taken != nil && taken(candidate))
			})}
			used[act.Copy] = true
		}
		actions = append(actions, act)
	}
	return actions
}

// conflictCopy is the path the cloud's version of a file is kept at: the file's own name with .conflict- and the time put
// before its extension, so an editor still opens it. It is the first such name that is free.
func conflictCopy(name string, now time.Time, exists func(string) bool) string {
	dir, file := path.Split(name)
	extension := path.Ext(file)
	stem := strings.TrimSuffix(file, extension)
	stamp := now.UTC().Format("20060102T150405Z")
	for n := 0; ; n++ {
		suffix := ".conflict-" + stamp
		if n > 0 {
			suffix += fmt.Sprintf("-%d", n)
		}
		candidate := dir + stem + suffix + extension
		if !exists(candidate) {
			return candidate
		}
	}
}

// Settled is the record of what both sides agree on after a plan has been carried out, which is the picture the next plan
// starts from. It is what the sides held when the plan was made, changed by what was done: so a path whose action failed keeps
// its old record, and is planned again the same way.
func Settled(base, local, remote Snapshot, done []Action) Snapshot {
	next := Snapshot{}
	for name, entry := range base {
		next[name] = entry
	}
	for _, act := range done {
		switch act.Kind {
		case Settle, Pull, Push:
			if act.Kind == Pull || act.Kind == Settle {
				next[act.Path] = remote[act.Path]
			}
			if act.Kind == Push {
				next[act.Path] = local[act.Path]
			}
		case DeleteLocal, DeleteRemote, Forget:
			delete(next, act.Path)
		case Conflict:
			// Both sides end with the local version at the name, and the cloud's at the copy.
			next[act.Path] = local[act.Path]
			next[act.Copy] = remote[act.Path]
		}
	}
	return next
}
