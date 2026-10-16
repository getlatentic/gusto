# The record of what changes on the platform

This record says what the people who run the platform did to it: who signed in, who was refused, who changed a role or a setup, and which settings changed. A job's record says what an agent did. An admin reads this one under Activity in the admin console, at `/admin/activity`.

## What is written

| Event | When |
| --- | --- |
| `signin`, `signout` | Somebody signs in (with the method) or out |
| `signin.refused` | A sign-in or an API call is turned away, and why |
| `role.changed` | Somebody signs in with a role that is not the one they last had |
| `setup.saved`, `skill.written`, `skill.dropped` | A skill set is created or its tools change, or a skill is added to it or removed |
| `token.created`, `token.revoked` | An admin makes or revokes an API token, with what it may do. The secret is never written |
| `member.invited`, `member.role`, `member.removed` | In an organisation behind a front door, an admin invites a person, changes their role or removes them. The front door reports these and the server writes them ([CLOUD.md](CLOUD.md)) |
| `budget.set` | An admin sets, changes or clears the monthly budget, with the figure before and after |
| `sandbox.confirmed` | A person on their own computer confirms, once, that agents run with no sandbox |
| `chat.default` | An admin sets or clears what a new chat runs on, with the engine and model before and after |
| `retention.hold`, `retention.release`, `retention.swept` | An admin keeps a job or a person's work from being removed, or lets it go. A sweep removed workspaces or records, with the counts and the records named |
| `server.started`, `settings.changed` | The server starts, with the settings that decide who may do what. One of those settings differs from the last start |

Every line has who, what, when and where the request came from. It never holds a password.

The settings are `GUSTO_ALLOWED`, `GUSTO_ADMINS`, `GUSTO_RUNNER_GROUPS`, `GUSTO_ADMIN_GROUPS`, `GUSTO_OIDC_GROUPS_CLAIM`, `GUSTO_OIDC_ISSUER`, `GUSTO_ALLOWED_HOSTS`, `GUSTO_MODELS`, `GUSTO_JOB_NETWORK`, `GUSTO_API_TOKENS`, `GUSTO_LIMIT_PERSON`, `GUSTO_LIMIT_TEAM`, `GUSTO_RETENTION`, and the hashes of the retention policy and the spend policy file. They change outside the pages, so `who` is `the server` and `when` is the next start.

A change is applied and then recorded. When the record cannot be written, the server says so in its log and the change stands. A source that causes more than ten refused sign-ins a minute has the rest counted and not written. The count goes on the next line written.

## Where it is

`GUSTO_CHANGES` names the file.

- Compose and the chart: `$GUSTO_DATA/changes/changes.jsonl` on the volume.
- The playbook: `/opt/gusto/var/changes/changes.jsonl`, append-only where the filesystem allows, so not even the server's user can rewrite or shorten it. The playbook says when it cannot.

An admin downloads the file from `/admin/activity.jsonl`. A script with the shared password, or with an API token that may read the change record, reads `/api/changes`. Both give the file as it is.

## Why an edit shows

The file is JSON lines. Each line has a number, the hash of the line before it, and its own hash. With a signing key, each line also carries the key's signature over its hash.

- An edited line no longer matches its hash.
- A removed, added or moved line breaks the numbering and the chain.
- A line rewritten with a fresh hash breaks the next line's link, and its signature does not verify.

The server will not start on a log that does not hold together. `GUSTO_KEYRING` names the keys behind the signatures, as for job records. A line signed by a key the keyring does not hold is counted, not refused.

## Check it

```bash
gusto changes verify changes.jsonl
# 6 events, and the chain holds
# head a24bf3dac7d4cdba764f6682feac035d2a0e5f117fcaa859acdafa7d3eaea617
#     6 signed by acme
# an edited line exits 1: the change log does not hold together at line 3: its content does not match its hash
```

## What no chain can show

Lines cut off the end leave a chain that still holds. Keep the head that `gusto changes verify` prints, in a place the server's machine cannot change (an export a reviewer holds is enough). Give it back later with `gusto changes verify -head <head> changes.jsonl`. When the head is not in the record, the command exits 1 and says lines were cut off the end, or it is another record.

Without a signing key the lines are chained and not signed, so anyone who can write the file can rewrite all of it. Set `GUSTO_SIGNING_KEY`, preferably a key in KMS ([KEYS.md](KEYS.md)).
