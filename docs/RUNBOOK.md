# Runbook

What to watch on a gusto server, and what to do when an alert fires.

## Health endpoints

| Path | Answers | Use it for |
| --- | --- | --- |
| `/healthz` | `ok` while the process runs | liveness probe. It never depends on the workers or the disk |
| `/readyz` | `ready`, or 503: fewer workers than `GUSTO_JOB_CONCURRENCY`, or the tasks directory is not writable | readiness probe, and the check a deploy makes before it keeps a new version |
| `/metrics` | Prometheus text format | scraping |

`/metrics` needs `Authorization: Bearer`. With API tokens ([TOKENS.md](TOKENS.md)), make a token that may read metrics at `/admin/tokens`. Without, use the shared password. Counters reset to zero on restart.

## Metrics

| Metric | Says |
| --- | --- |
| `gusto_build_info{release}`, `gusto_started_timestamp_seconds` | the release, and when it started |
| `gusto_jobs{state}`, `gusto_jobs_finished_total{state}` | tasks held, and tasks ended since start, by `queued`, `running`, `passed`, `failed`, `interrupted` |
| `gusto_queue_depth`, `gusto_queue_oldest_wait_seconds` | tasks waiting, and the longest wait |
| `gusto_queue_stuck_seconds` | the longest wait of a task that nothing holds back (no limit, no busy agent, a runner free). Near zero while the queue moves |
| `gusto_jobs_running`, `gusto_runner_slots`, `gusto_runner_workers_alive` | tasks running, tasks allowed at once, workers alive. Fewer workers than slots means the queue is broken. `gusto_last_job_started_timestamp_seconds` and `gusto_last_job_finished_timestamp_seconds` give the last start and finish, 0 before the first |
| `gusto_disk_free_bytes{store="jobs"}`, `gusto_disk_total_bytes{store="jobs"}` | the volume the tasks are on |
| `gusto_jobs_disk_bytes`, `gusto_jobs_workspace_bytes`, `gusto_oldest_finished_job_timestamp_seconds` | what tasks take (measured at most once a minute), the workspaces among them, and when the longest-kept finished task ended |
| `gusto_proxy_calls_total`, `gusto_proxy_errors_total`, `gusto_proxy_refused_total` | model calls; calls the model's side failed (no answer, or a 5xx); calls a spending cap stopped. Absent with no proxy |
| `gusto_last_deploy_ok{release}`, `gusto_last_deploy_timestamp_seconds` | 1, or 0 for a failed or rolled-back deploy. Present where a deploy writes `GUSTO_DEPLOY_RESULT`, as the playbook's autodeploy does |

## Alerts

The rules are in `deploy/alerts/gusto.rules.yml`, with `promtool` tests in `gusto.rules.test.yml` and a scrape in `prometheus.example.yml`. Each rule's `runbook` annotation points at its heading here.

### GustoDown

Prometheus could not scrape `/metrics` for two minutes.
1. Run `curl -sS https://<host>/healthz`. If it answers and `/metrics` gives 401, make a new token at `/admin/tokens`.
2. If nothing answers, run `systemctl status gusto` and `journalctl -u gusto -n 100`. The server refuses to start on a change record that does not hold together, and on data a newer release wrote ([UPGRADE.md](UPGRADE.md)). If the last deploy broke it, see GustoDeployFailed.

### GustoNotReady

Fewer workers run than `GUSTO_JOB_CONCURRENCY`. `/readyz` gives 503. Run `systemctl restart gusto`. Running tasks end and are marked interrupted. If it recurs, read the log for the error that took a worker.

### GustoQueueStuck

A queued task that nothing holds back has waited over five minutes. The log says `could not claim a task` when the tasks directory is not writable.
1. Run `df -h` on the tasks volume, and `journalctl -u gusto | grep "could not claim"`.
2. Free the disk (see GustoDiskLow) or fix the directory's permissions. If the log shows nothing, run `systemctl restart gusto`.

### GustoRunnersSaturated

Every runner has been busy, with tasks waiting, for half an hour. Nothing is broken. Check Activity.
- One person takes every runner: set `GUSTO_LIMIT_PERSON` ([LIMITS.md](LIMITS.md)).
- Too little capacity: raise `GUSTO_JOB_CONCURRENCY` if memory allows (`GUSTO_JOB_MEMORY` times the concurrency must fit), or add a machine.
- A task hangs: shorten its budget in seconds.

### GustoJobsFailing

More than half the tasks that ended in the last hour failed, and at least five did.
1. Open a few failed tasks from Activity. Each page says whether the check failed, the agent exited, or a refusal came.
2. If the failures match, suspect what they share: a repository the GitHub App cannot reach ([REPOS.md](REPOS.md)), an expired model credential, a changed runner image.
3. Run `gusto doctor`.

### GustoDiskLow

Less than a tenth of the tasks volume is free. Tasks fail to write and the queue stalls. Open `/admin/records?tab=keeping`. Set or shorten the policy ([RETENTION.md](RETENTION.md)), then sweep. `gusto retention` gives a dry run. Run `du` to find other things on the same volume.

### GustoModelErrors

More than a fifth of model calls in ten minutes failed on the model's side (no answer, or a 5xx), with at least ten calls. Check the provider's status. Read `journalctl -u gusto` and a failed task's page for the provider's error. An expired credential gives 4xx, which this alert does not count.

### GustoDeployFailed

The last deploy failed or rolled back, and the server runs the earlier version. The deploy's GitHub workflow run says which step failed.
1. Read `journalctl -u 'gusto-deploy@*'` on the box for the play's output. A smoke failure means the candidate could not run a task. A refusal from `gusto data check` means the data would not upgrade ([UPGRADE.md](UPGRADE.md)). A rollback means the new version did not answer `/readyz`.
2. Fix on a branch and merge. A merge to `main` deploys again, and a good deploy clears the alert.

### GustoRestartingOften

The server started three times in half an hour: a crash loop, or a deploy that keeps restarting it. Run `journalctl -u gusto` to see each exit.
