# Retention

Finished work is removed on a schedule the operator sets, so that a busy team
does not fill the disk. A job keeps two things for different lengths of time:

| What | Holds | Removed |
| --- | --- | --- |
| the workspace | the checkout and everything the agent changed. It is most of the disk, and `gusto ship` and `gusto replay` need it | first |
| the record | `out/`: the manifest, its signature, the patch, the transcript. It is small, and it is what `gusto verify` checks | later |

Removing a workspace leaves the record whole. The manifest lists the files in
`out/` and never the workspace, so `gusto verify` still says the record holds and
who signed it. A job with no workspace cannot be shipped or replayed.

## Set the policy

Days are counted from when the job ended, by the state it ended in: `passed`,
`failed` or `interrupted`. A state with no entry, or zero, is kept for ever.

```json
{
  "workspaceDays": { "passed": 14, "failed": 30, "interrupted": 7 },
  "recordDays":    { "passed": 365, "failed": 365, "interrupted": 90 }
}
```

A record cannot be set to go before its workspace, and a state a job cannot end
in is refused. So is a misspelt field, because the policy would keep everything.
The server will not start on a policy it refuses.

| Install | Where |
| --- | --- |
| Playbook | `retention_policy` in the host's variables |
| Chart | `retention` in the values |
| Compose | `policy.json` in `$GUSTO_DATA/retention/` on the volume |

`GUSTO_RETENTION` names the directory. It holds `policy.json`, `holds.json` and
`last.json`, the newest sweep's report. With no policy the server removes
nothing, and `/admin/retention` still shows the disk and the holds.

## What a sweep touches

The server sweeps every 60 minutes, and an admin can sweep at once from
`/admin/retention`. `GUSTO_RETENTION_MINUTES` changes the interval.

A sweep touches only jobs that ended and that no agent owns. A task or a message
on an agent belongs to that agent's conversation, and goes when the agent is
removed. A job that is queued or running is never touched.

## Holds

A hold keeps work whole whatever the policy says. An admin holds one job by its
id, or everything a person asked for, on the Retention page of the admin console. A hold on a
person covers work they ask for later too.

A sweep that cannot read the holds removes nothing.

## Seeing it

`/admin/retention` shows the disk the jobs use, how much of it is workspaces, the
oldest finished job kept, the policy in force, what the next sweep would
remove, and the last sweep.

```bash
gusto retention -dir /opt/gusto/var/retention -jobs /opt/gusto/jobs
```

lists what a sweep would remove now and removes nothing. It reads the jobs from
disk and does not start anything, so it is safe to run while the server does.

## What is written down

Holds, releases and each sweep that removed something go in the change record
([CHANGES.md](CHANGES.md)). A sweep's line counts the workspaces and the records
it removed and names the records, up to 200; `last.json` names them all. The
policy's hash is one of the settings the record watches.
