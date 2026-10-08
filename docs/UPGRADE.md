# Upgrading and rolling back

A release never costs you conversations, jobs or records. Every directory the
server keeps data in carries a format, a newer release migrates it, and an older
release that meets data it cannot read refuses by name and changes nothing.

## The formats

Each store has a format number, written to `.haas-format` in its directory:

| Store | Directory | Holds |
| --- | --- | --- |
| jobs | `HAAS_JOBS` | each job's directory, with its signed record |
| sessions | `HAAS_SESSIONS` | agents, their threads and transcripts |
| setups | `HAAS_SETUPS` | skill sets |
| comparisons | `HAAS_COMPARISONS` | the comparisons started from the pages |
| changes | the directory of `HAAS_CHANGES` | the change record |
| tokens | the directory of `HAAS_API_TOKENS` | API tokens |
| retention | `HAAS_RETENTION` | the policy, the holds and the last sweep |

A directory with data and no stamp was written before stamps existed, and is
format 1. Every store is at format 1 in this release. The stamp names the release
that last wrote it, and no release before stamps lists it, so every release still
reads it: the file's name begins with a dot for that reason.

Files inside the stores have their own versions, which
[FORMATS.md](FORMATS.md) describes. A reader refuses a version newer than it
knows, by name. The store format is the version of the directory as a whole.

## Upgrade

Start the new release. Before it opens anything it looks at every directory:

- one at an older format is migrated, a step at a time, and stamped after each
  step. A step that fails leaves the stamp where it was, and the same start runs
  it again;
- one at a format newer than the release reads is refused, naming the directory,
  the format and the release that wrote it;
- one whose migration cannot be undone is refused until you say it may go ahead.

Every directory is checked before any is changed, so a refusal from one leaves
the others as they were. The server does not start after a refusal.

To see what a start would do without starting:

```bash
haas data check     # each store: nothing to do, or the steps and whether each can be undone
haas data status    # each store's format and the release that wrote it
haas version
```

`haas data check` exits 1 when the server would refuse. The playbook runs it with
the new binary before that binary is promoted, so a refusal fails the deploy
while the running version keeps serving.

## An upgrade that cannot be undone

Some migrations lose what the old format held. They are refused with the steps
named. Copy the directories first, then set `HAAS_MIGRATE_IRREVERSIBLE=1` (the
playbook's `migrate_irreversible`) for that one start, and turn it off again.
After it runs, an older release cannot read that data, and going back means
restoring the copy.

## Roll back

Go back with the newer release still installed, because only it knows how to
undo what it did:

```bash
haas data rollback -to v0.1.0-rc.1
```

It checks every store first. If one has a migration that cannot be undone, it
says which, and changes nothing. Otherwise each store is taken back to the
format that release wrote, and that release can run on the data. Install it and
start it.

Rolling back does not undo what people did in the meantime: jobs and messages
made under the new release stay, in the older format.

## Releases

What each release wrote, and how to go back to it. A release that changes a
format adds itself here and to `internal/datadir/releases.go`.

| Release | Formats | Going back |
| --- | --- | --- |
| v0.1.0-rc.1 | every store at format 1, unstamped | `haas data rollback -to v0.1.0-rc.1`. Nothing to undo: no release has changed a format |
| v0.0.1-alpha.1 | every store at format 1, stamped with the release | `haas data rollback -to v0.0.1-alpha.1`. Nothing to undo: no release has changed a format |
| v0.0.1-alpha.2 | every store at format 1, stamped with the release | `haas data rollback -to v0.0.1-alpha.2`. Nothing to undo: no release has changed a format |
| v0.0.1-alpha.3 | every store at format 1, stamped with the release | `haas data rollback -to v0.0.1-alpha.3`. Nothing to undo: no release has changed a format |
| v0.0.1-alpha.4 | every store at format 1, stamped with the release | `haas data rollback -to v0.0.1-alpha.4`. Nothing to undo: no release has changed a format |

## Rehearse it on your data

```bash
deploy/rehearse-upgrade.sh ./haas /var/lib/haas v0.0.1-alpha.2
```

It copies your data directory and never changes the original. On the copy it
shows what the new binary would migrate, starts it as a server, loads each job's
page, and runs `haas verify` on every record. With a release named, it then rolls
the copy back to that release's formats. It exits 1 and says which job when a page
does not load or a record does not verify.

## Checked on every change

`deploy/upgrade-check.sh` builds each of the last two release tags, has it make a
signed job and the server's directories, upgrades the data with the commit under
test, and reads the job back: the page loads and `haas verify` holds. It then
rolls back and starts the old release on the same data, and reads the job again.
The `upgrade-check` workflow runs it on every pull request that touches the code.

```bash
deploy/upgrade-check.sh v0.1.0-rc.1
```

## Changing a format

A change that leaves every existing file readable is not a new format. A change
that makes an old release misread what a new one wrote is one:

1. raise the store's number in `internal/datadir/datadir.go`;
2. add a `Step` in `internal/datadir/steps.go` from the old number, with `Up`
   and, if the change can be undone, `Down`. `Up` must be safe to run twice;
3. add the release to `Releases`, and its row to the table above;
4. test the step on data the previous release made.

A test fails when a format has no step out of it, or when a release lists a
format this one does not have.
