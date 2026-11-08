# A turn that a restart interrupted

Decision for getlatentic/planning#591. Nothing here is built.

## The decision

- A turn that a restart interrupted is not resumed and is not run again by the server.
- The next turn in the same thread is told that the one before was interrupted, and is given what the record holds of it. That is the one piece worth building next.
- An automatic rerun waits until a turn can say, from its own record, that it did nothing outside the working folder. Nothing a harness prints says that today.
- A record never says a turn "continued". An interrupted turn has no manifest, and the turn that follows it is a new turn with its own manifest.

## What a restart leaves today

A turn is a task whose spec names a `session` (the instance), a `thread` and `remembered` (the agent's own name for the conversation). When the server starts, `tasks.Open` finds `state.json` saying `running`. With no `out/manifest.json` written after the task started, it ends the task `interrupted` with exit -1 (`internal/tasks/index.go`, `ended`). With a manifest it grades the task as the run would have.

An interrupted turn leaves:

| Kept | Why it is there |
| --- | --- |
| The edits the agent made | A turn works in the instance's own folder, so they are not in a throwaway clone |
| `out/transcript.log`, `out/harness.log` and the other lines the recorder wrote | Each line reaches the disk before the next is read |
| `state.json` as `interrupted` | Written by recovery |

It does not leave `out/diff.patch`, the check's answer or the manifest, which are written after the agent stops. The tree the diff would be taken against is in `gusto-trees-*` under the temporary directory and is removed by the diff, so a crash leaves it behind with nothing that names it.

The agent's own conversation name is the gap. `Recorder.Remembered` holds it while the turn runs and the runner writes it to the manifest at the end (`end.session`) and nowhere else. A turn that was interrupted has no manifest, so nothing holds its name, and the thread keeps the name from the turn before. (The code that copies the name from a finished turn into the thread is outside the packages this note was written from, and was not read.) The agent's own history of the interrupted turn exists in its process's files but gusto does not know which one it is.

## The three questions

**Which harnesses can resume a conversation after their process died.** Read from the adapters and `internal/acp`, not run against each program:

| Harness | Stream | How it takes up a conversation |
| --- | --- | --- |
| claude, codex, hermes, dsh | ACP | `session/resume`, else `session/load`, when the agent advertises one. Otherwise `session/new`. A named session the agent cannot find fails the turn (`acp.open`) |
| opencode, pi, muse | JSONL | a `resume` argument list in the adapter, with the session id substituted |

So the mechanism exists and is already used between turns. What is missing for an interrupted turn is the session id (above), and a harness-side check that its stored conversation still holds the half turn. That check has not been done for any harness. The agent's state directory (`keeps` in the adapter) has to survive the restart as well, which it does on a person's laptop and in a node with its disk, and does not in a box that was thrown away.

**What an interrupted turn leaves, and whether a rerun is safe.** The working folder holds a partial change. Running the same brief on it asks the agent to continue from a folder it did not see being made, which is what a person does when they type "carry on". That is safe for the folder. It is not known to be safe for anything outside it: a pushed branch, a posted message, a paid call, a ticket opened. Gusto runs unmodified harnesses, so it cannot see inside a tool call, and a signed record shows only that files were not edited after the turn. It does not show that an outside effect did not happen, so a rerun that repeats one is not safe, and the record cannot vouch for one. Pi Durable can rerun because each tool declares `replay: "safe"`. The tools here are the harness's own and carry no such declaration.

**Whether "interrupted, and the next turn is told so" is enough.** Yes, as the first step and for now as the only one. It costs one extra paragraph at the start of the next turn and the person decides, in the thread, whether to say "carry on". It never repeats an effect on its own. The person who sees `interrupted` in the list already knows the turn did not finish; the gap is that the agent does not.

## What building the next step takes

1. Write the agent's conversation name to `state.json` the moment the recorder sees it, not at the end, so recovery can hand it on.
2. At recovery, keep a short note on the task: interrupted by a restart, with the last lines of `out/transcript.log`.
3. When the next turn in the thread starts, put the note in front of the brief. The note is part of the turn's input and is therefore in the record.
4. Take the diff of the folder against the last turn's end as well, if the instance keeps one, so the agent is told which files are half made.

An automatic rerun can be reconsidered when an adapter can name the tools whose calls only read, and the transcript shows no other kind ran in the interrupted turn. That is a per-harness claim and needs its own measurement.

## Not tested

No harness was started, stopped and resumed for this decision. The table of resume mechanisms is what the adapters and `internal/acp` say they do.
