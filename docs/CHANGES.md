# The record of what changes on the platform

A job's record says what an agent did. This record says what the people who run
the platform did to it: who signed in, who was refused, who changed a role or a
setup, and which settings changed. An admin reads it under Activity in the admin console, at `/admin/activity`.

## What is written

| Event | When |
| --- | --- |
| `signin` | somebody signs in, with the method |
| `signin.refused` | a sign-in or an API call is turned away, and why |
| `signout` | somebody signs out |
| `role.changed` | somebody signs in with a role that is not the one they last had |
| `setup.saved` | a skill set is created or its tools change |
| `skill.written`, `skill.dropped` | a skill is added to a skill set or removed |
| `token.created`, `token.revoked` | an admin makes or revokes an API token, with what it may do. The secret is never written |
| `member.invited`, `member.role`, `member.removed` | in an organisation behind a front door, an admin invites a person, changes their role or removes them. The front door reports these and the server writes them ([CLOUD.md](CLOUD.md)) |
| `budget.set` | an admin sets, changes or clears the monthly budget, with the figure before and after |
| `sandbox.confirmed` | a person on their own computer confirms, once, that agents run with no sandbox |
| `chat.default` | an admin sets or clears what a new chat runs on, with the engine and model before and after |
| `retention.hold`, `retention.release` | an admin keeps a job or a person's work from being removed, or lets it go |
| `retention.swept` | a sweep removed workspaces or records, with the counts and the records named |
| `server.started` | the server starts, with the settings that decide who may do what |
| `settings.changed` | one of those settings differs from the last start |

Every line has who, what and when, and where the request came from. It never
holds a password: a wrong password is recorded as wrong, not as typed.

The settings are `HAAS_ALLOWED`, `HAAS_ADMINS`, `HAAS_RUNNER_GROUPS`,
`HAAS_ADMIN_GROUPS`, `HAAS_OIDC_GROUPS_CLAIM`, `HAAS_OIDC_ISSUER`,
`HAAS_ALLOWED_HOSTS`, `HAAS_MODELS`, `HAAS_JOB_NETWORK`, `HAAS_API_TOKENS`,
`HAAS_LIMIT_PERSON`, `HAAS_LIMIT_TEAM`, `HAAS_RETENTION`, and the hashes of the
retention policy and of the spend policy file. They change outside the pages, so `who` is `the server` and
`when` is when it next started.

A change is applied and then recorded. When the record cannot be written, the
server says so in its log and the change stands.

A refused sign-in is anybody's to cause, so a source that causes more than ten a
minute has the rest counted and not written. The count goes on the next line
that is written.

## Where it is

`HAAS_CHANGES` names the file. Compose and the chart keep it at
`$HAAS_DATA/changes/changes.jsonl` on the volume, and the playbook at
`/opt/haas/var/changes/changes.jsonl`. The playbook makes it append-only where
the filesystem allows, so that not even the user the server runs as can rewrite
or shorten it, and says when the filesystem would not.

An admin downloads it from `/admin/activity.jsonl`. A script with the shared password,
or with an API token that may read the change record, reads `/api/changes`. Both give the file as it is.

## Why an edit shows

The file is JSON lines. Each line has a number, the hash of the line before it,
and its own hash. When the deployment holds a signing key, each line also carries
the key's signature over its hash.

- An edited line no longer matches its hash.
- A removed, added or moved line breaks the numbering and the chain at that line.
- A line rewritten with a fresh hash breaks the next line's link, and its
  signature does not verify, because only the key can make one.

The server will not start on a log that does not hold together, because what it
went on to write would look as sound as what came before.

## Check it

```bash
haas changes verify changes.jsonl
```

```
6 events, and the chain holds
head a24bf3dac7d4cdba764f6682feac035d2a0e5f117fcaa859acdafa7d3eaea617
    6 signed by acme
```

An edited line stops it and exits 1:

```
the change log does not hold together at line 3: its content does not match its hash
```

`HAAS_KEYRING` names the keys that stand behind the signatures, as it does for
job records. A line signed by a key the keyring does not hold is counted, not
refused.

## What no chain can show

Lines cut off the end of the file leave a chain that still holds. Keep the head
that `haas changes verify` and the page print, and give it back later:

```bash
haas changes verify -head a24bf3dac7d4cdba764f6682feac035d2a0e5f117fcaa859acdafa7d3eaea617 changes.jsonl
```

```
the head a24bf3dac7d4cdba764f6682feac035d2a0e5f117fcaa859acdafa7d3eaea617 is not in this record: lines were cut off the end, or it is another record
```

It exits 1. Keep the head somewhere the server's own machine cannot change: an
export a reviewer holds is enough.

Without a signing key the lines are chained and not signed. Somebody who can
write the file can then rewrite all of it. Set `HAAS_SIGNING_KEY`, and prefer a
key in KMS ([KEYS.md](KEYS.md)), so the key is not on the machine that could be
broken into.

## What is not here

A job's activity, spend and evidence are in the jobs' own records. The record of
what changes on the platform does not hold them, and a job's record does not hold
this.
