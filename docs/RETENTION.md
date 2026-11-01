# Retention

The server removes finished work on a schedule you set, so that a busy team does not fill the disk. A task keeps two things for different lengths of time.

| What | Holds | Removed |
| --- | --- | --- |
| the workspace | the checkout and everything the agent changed. It is most of the disk. `gusto pr open` and `gusto rerun` need it | first |
| the record | `out/`: the manifest, its signature, the patch, the transcript. It is small. `gusto verify` checks it | later |

Removing a workspace leaves the record whole, and `gusto verify` still passes. A task with no workspace cannot open a pull request or be rerun.

## Set the policy

Days count from when the task ended, by the state it ended in: `passed`, `failed` or `interrupted`. A state with no entry, or zero, is kept for ever.

```json
{
  "workspaceDays": { "passed": 14, "failed": 30, "interrupted": 7 },
  "recordDays":    { "passed": 365, "failed": 365, "interrupted": 90 }
}
```

The server refuses a policy that sets a record to go before its workspace, names a state a task cannot end in, or has a misspelt field. It does not start on a refused policy.

| Install | Where |
| --- | --- |
| Playbook | `retention_policy` in the host's variables |
| Chart | `retention` in the values |
| Compose | `policy.json` in `$GUSTO_DATA/retention/` on the volume |

`GUSTO_RETENTION` names the directory. It holds `policy.json`, `holds.json` and `last.json`, the newest sweep's report. With no policy the server removes nothing, and `/admin/records?tab=keeping` still shows the disk and the holds.

## Sweeps

The server sweeps every 60 minutes. Set `GUSTO_RETENTION_MINUTES` to change the interval. An admin can sweep at once from `/admin/records?tab=keeping`.

A sweep touches only tasks that have ended and that no agent owns. A task or message on an agent goes when the agent is removed. A queued or running task is never touched.

## Holds

A hold keeps work whole whatever the policy says. On the Retention page of the admin console, an admin holds one task by its id, or everything a person asked for. A hold on a person covers work they ask for later.

A sweep that cannot read the holds removes nothing.

## See what a sweep removes

`/admin/records?tab=keeping` shows the disk the tasks use, how much is workspaces, the oldest finished task kept, the policy in force, what the next sweep would remove, and the last sweep.

This command lists what a sweep would remove now and removes nothing. It is safe to run while the server runs.

```bash
gusto retention -dir /opt/gusto/var/retention -tasks /opt/gusto/jobs
```

## Change record

Holds, releases and each sweep that removed something go in the change record ([CHANGES.md](CHANGES.md)). A sweep's line counts what it removed and names up to 200 records. `last.json` names them all.
