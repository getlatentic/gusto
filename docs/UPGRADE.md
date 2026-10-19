# Upgrading and rolling back

A release never costs you conversations, tasks or records. Each data directory carries a format, a newer release migrates it, and an older release that meets data it cannot read refuses by name and changes nothing.

## The formats

Each store has a format number in `.gusto-format` in its directory. A directory with data and no stamp is format 1. Every store is at format 1 in this release. Files inside the stores have their own versions ([FORMATS.md](FORMATS.md)); a reader refuses a version newer than it knows.

| Store | Directory | Holds |
| --- | --- | --- |
| tasks | `GUSTO_JOBS` | each task's directory, with its signed record |
| sessions | `GUSTO_SESSIONS` | agents, their threads and transcripts |
| setups | `GUSTO_SETUPS` | skill sets |
| comparisons | `GUSTO_COMPARISONS` | comparisons started from the pages |
| integrations | `GUSTO_INTEGRATIONS` | plugins an administrator imported, a folder for each version |
| presets | `GUSTO_PRESETS` | what each person saved to start a chat again, a file each |
| changes | the directory of `GUSTO_CHANGES` | the change record |
| tokens | the directory of `GUSTO_API_TOKENS` | API tokens |
| retention | `GUSTO_RETENTION` | the policy, the holds and the last sweep |

## Upgrade

Start the new release. Before it opens anything it checks every directory:

- One at an older format is migrated a step at a time, and stamped after each step. A failed step leaves the stamp, and the same start runs it again.
- One at a newer format than the release reads is refused, naming the directory, the format and the release that wrote it.
- One whose migration cannot be undone is refused until you allow it.

No directory is changed before all are checked, and the server does not start after a refusal. To see what a start would do:

```bash
gusto data check     # each store: nothing to do, or the steps and whether each can be undone
gusto data status    # each store's format and the release that wrote it
gusto version
```

`gusto data check` exits 1 when the server would refuse. The playbook runs it with the new binary before promoting it, so a refusal fails the deploy while the running version keeps serving.

To allow a migration that cannot be undone, copy the directories first. Set `GUSTO_MIGRATE_IRREVERSIBLE=1` (the playbook's `migrate_irreversible`) for that one start, then turn it off. After it runs, an older release cannot read that data, and going back means restoring the copy.

## Roll back

Keep the newer release installed, because only it can undo what it did:

```bash
gusto data rollback -to v0.1.0-rc.2
```

It checks every store first. If one has a migration that cannot be undone, it names the store and changes nothing. Otherwise it takes each store back to the format that release wrote. Install that release and start it. Tasks and messages made under the newer release stay, in the older format.

The release v0.1.0-rc.2 wrote every store at format 1, so rolling back to it (`gusto data rollback -to <release>`) undoes nothing. A release that changes a format adds itself to `internal/datadir/releases.go`.

## Rehearse it on your data

```bash
deploy/rehearse-upgrade.sh ./gusto /var/lib/gusto v0.1.0-rc.2
```

It never changes the original. On a copy of your data directory it shows what the new binary would migrate, starts a server, loads each task's page, and runs `gusto verify` on every record. With a release named, it then rolls the copy back to that release's formats. It exits 1 and names the task when a page does not load or a record does not verify.

## Changing a format

A change that makes an old release misread what a new one wrote is a new format:

1. Raise the store's number in `internal/datadir/datadir.go`.
2. Add a `Step` in `internal/datadir/steps.go` from the old number, with `Up` (safe to run twice) and, if the change can be undone, `Down`.
3. Add the release to `Releases`.
4. Test the step on data the previous release made.

A test fails when a format has no step out of it, or when a release lists a format this one does not have.
