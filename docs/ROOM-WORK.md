# Agents working at the same time in a room

Two agents mentioned in one message run at the same time. Each works on its own copy of the room's folder. What a turn changed is merged into the room's folder when the turn passes.

## The choice

A room's agents share one folder. Two ways to let them write to it at once were tried in a spike:

| Way | What it is | Verdict |
| --- | --- | --- |
| Locks on the files a turn writes | A turn takes a lock on a file before it writes it | Not possible. An agent writes with its own shell and editor tools, so the server cannot know which file comes next. A lock on the folder is the queue that existed before. A lock taken after the write is a warning, not a lock |
| A git worktree for each turn | `git worktree add` for the turn, merge with git | Not used. The room's folder is not kept committed, so a worktree holds only the committed files and loses what people and agents wrote since. The folder's `.git` is the agent's to write, and git runs the programs its configuration names. Two agents that change one file give a text conflict that nobody is there to resolve |
| A plain copy for each turn, merged back | Copy the folder, run the turn on the copy, put the changed files back | Chosen |

A copy keeps every file the room has, committed or not. The agent runs against an ordinary folder, in a container, in a sandbox or in this process. The merge compares files and runs no program from the folder.

## What the spike measured

Each run is on an Apple M1 Max with APFS. The three folders hold 200 files of 10 KB (2 MB), 5,000 files of 20 KB (98 MB) and 30,000 files of 20 KB (586 MB).

| What | 200 files | 5,000 files | 30,000 files |
| --- | --- | --- | --- |
| Copy each file in Go | 188 ms | 1.95 s | 28.3 s |
| `cp -R` | 421 ms | 4.5 s | 20.9 s |
| `cp -c -R`, a clone that shares data blocks | 257 ms | 2.2 s | 6.2 s |
| `git worktree add` | 127 ms | 1.35 s | 5.4 s |
| Walk and `stat` every file | 19 ms | 94 ms | 203 ms |
| Read and hash every file | 148 ms | 2.07 s | 6.99 s |

Reading every file to find what changed costs as much as the copy. So the merge finds the files a turn changed from their `stat`: size, modified time, mode, and the change time and inode where the platform has them. It reads only the files that differ.

The finished package, `internal/roomwork`, measured on the same folders (`go test ./internal/roomwork -run '^$' -bench Turn -benchtime 3x`):

| What | 200 files | 5,000 files | 30,000 files |
| --- | --- | --- | --- |
| Begin: copy the folder, clone where the file system can | 91 ms | 1.3 s | 7.3 s |
| Merge a turn that changed 10 files, scan included | 22 ms | 68 ms | 319 ms |

A model call takes seconds, and a turn takes tens of seconds. The copy costs a small part of a turn for a folder of ordinary size. The cost grows with the number of files, not their size.

Two agents of 4 seconds each, mentioned in one message, on a local server with two workers and a canned agent:

| | Wall time |
| --- | --- |
| Before: one after the other | 8.4 s to 8.5 s |
| After: at the same time | 4.3 s to 4.4 s |

## What a turn does

1. The queue starts a turn when no other turn of the same agent in the room is running (see Waiting).
2. The turn copies the room's folder into its own task directory. Where the file system can share blocks (APFS, btrfs, xfs) the copy shares them. Elsewhere it copies the bytes. Links are copied as links and never followed.
3. The agent runs on the copy. The turn's own record (`out/diff.patch`) says what this turn changed.
4. When the turn passed, the changed files are merged into the room's folder. The copy is then emptied.
5. A turn that failed, or was stopped, is emptied and merged into nothing. The room's folder is as it was.

`.git` is copied, so the agent can use git. It is never merged. A commit a turn made stays in its copy, and the files the commit holds reach the folder as changes in the working tree.

## The merge

The merge compares three things for each file the turn changed: the file as the turn's copy began, the turn's file now, and the room's file now.

| The turn | The room's file | What happens |
| --- | --- | --- |
| Changed it | Not changed since the turn began | The turn's file is put there |
| Changed it | Changed since, to the same content | Nothing |
| Changed it | Changed since, to other content | A conflict |
| Wrote it again with the same content | Changed since | Nothing. A formatter that rewrites every file does not conflict with another agent's change |
| Deleted it | Not changed since | It is deleted, and a folder the deletion empties is removed |
| Deleted it | Changed since | It stays, and the room says so |
| Made it | Made since, with other content | A conflict |

A person's upload counts as a change since the turn began, as another agent's merge does.

### A conflict

The folder keeps the version that arrived first. The later version is saved beside it with the agent's name before the extension: `report.md` and `report.bea.md`. A second one for the same file is `report.bea-2.md`. If the file's folder is in the way, the version is saved at the top of the folder as `docs-a.bea.md`.

The room shows a note, kept in the conversation like any other note, and the agents are told it when they next speak:

> Ada and Bea both changed report.md. report.md is Ada's version and report.bea.md is Bea's version.

Where the version already in the folder came from a person, the note says "while somebody else did". A note names five files at most and counts the rest.

### All or nothing

A merge stages every file beside the folder, keeps each file it replaces or removes, and writes a journal. A failure puts every file back as it was. A server that dies part way leaves the journal, and the next turn in the room undoes the merge before it begins. A file that is replaced is replaced by one rename, so a reader never finds it missing.

One merge changes the room's folder at a time. It takes the changed files of one turn, and a stopped turn does not wait for it.

## Waiting

The queue keeps turns apart by *lane*, not by room. A room's agent keeps one conversation and one memory, so its own turns go one at a time, in the order they were asked. Two agents of a room are two lanes and run beside each other. The lane of a turn that is not a room's is its conversation, as before.

A turn that has not started says so, and says what holds it:

| Message | Cause |
| --- | --- |
| Waiting for Ada to finish the message before this one. Number 2 in line. | the same agent is on another message |
| Every runner is busy: 2 of 2. It starts when one is free. Number 3 in line. | `GUSTO_JOB_CONCURRENCY` turns are running |
| The person or team limit, in [LIMITS.md](LIMITS.md) | a limit |

"Working" is for a turn an agent is on. The room's page, a conversation's page and a comparison's line all say it this way.

Two agents run at the same time only when the server runs two tasks at the same time. `GUSTO_JOB_CONCURRENCY` is 1 by default. Set it to the number of agents that may run at once, with `GUSTO_JOB_MEMORY` times that fitting the machine.

## An agent's memory

An agent keeps what it remembers of a conversation in its own state folder, `agents/<id>/state` in the room. The first time an agent runs after this change, its folder is a copy of the room's shared `state`, so the conversation it had carries on. After that only that agent's turns write to it, and they go one at a time.

## What it costs

- Every turn of a room copies the folder. Over `GUSTO_ROOM_COPY_MB` (4096 by default) or `GUSTO_ROOM_COPY_FILES` (200,000) a turn is refused, and its record says so. Take files out of the room, or raise the limits.
- Each running turn holds a copy. Where the file system cannot share blocks, that is the folder's size on disk for each turn that runs.
- A file two agents changed needs a person to pick one.
- A commit a turn made is not in the room's `.git`.
- A merge that finds a link or a file where a folder of the turn's goes saves the turn's file at the top of the folder, never through the link.
- A person's upload that lands in the instant a merge replaces the same file can be replaced. The merge keeps the file it replaced until the merge ends, and the room's folder is not locked against uploads.

## Where it is

| Part | Where |
| --- | --- |
| The copy, the merge, the journal | `internal/roomwork` |
| The lane | `Record.Lane` in `internal/jobs/state.go` |
| Running a turn on a copy, in a container, a sandbox or this process | `cmd/gusto/roomturn.go` |
| The note | `Store.Notice` in `internal/instance/room.go`, shown by `internal/web/templates/conversation.html` |
| What a waiting turn says | `internal/web/rooms_waiting.go` |
