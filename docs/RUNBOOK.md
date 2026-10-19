# Runbook

What to watch on a gusto server, and what to do when an alert fires.

## Health endpoints

| Path | Answers | Use it for |
| --- | --- | --- |
| `/healthz` | `ok` while the process runs | a liveness probe. It never depends on the workers or the disk |
| `/readyz` | `ready`, or 503 saying what is wrong: fewer workers running than `GUSTO_JOB_CONCURRENCY`, or the jobs directory cannot be written | a readiness probe, and the check a deploy makes before it keeps a new version |
| `/metrics` | the Prometheus text format | scraping |

`/metrics` needs `Authorization: Bearer`. On a server with API tokens
([TOKENS.md](TOKENS.md)), make a token that may **read metrics** at `/admin/tokens`.
On a server without, the shared password is the credential.

## Metrics

| Metric | Type | Says |
| --- | --- | --- |
| `gusto_build_info{release}` | gauge | the release this server is |
| `gusto_started_timestamp_seconds` | gauge | when it started |
| `gusto_jobs{state}` | gauge | jobs held, by `queued`, `running`, `passed`, `failed`, `interrupted` |
| `gusto_queue_depth` | gauge | jobs waiting to start |
| `gusto_queue_oldest_wait_seconds` | gauge | how long the longest-waiting job has waited |
| `gusto_queue_stuck_seconds` | gauge | how long the longest-waiting job has waited that **nothing holds back**: no limit, no busy agent, a runner free. Near zero while the queue moves |
| `gusto_jobs_running` | gauge | jobs running now |
| `gusto_runner_slots` | gauge | how many jobs run at once |
| `gusto_runner_workers_alive` | gauge | workers running. Fewer than the slots means the queue is broken |
| `gusto_jobs_finished_total{state}` | counter | jobs that ended since the server started |
| `gusto_last_job_started_timestamp_seconds`, `gusto_last_job_finished_timestamp_seconds` | gauge | since the server started; 0 before the first |
| `gusto_disk_free_bytes{store="jobs"}`, `gusto_disk_total_bytes{store="jobs"}` | gauge | the volume the jobs are on |
| `gusto_jobs_disk_bytes`, `gusto_jobs_workspace_bytes` | gauge | what the jobs take, and the workspaces among them. Measured at most once a minute |
| `gusto_oldest_finished_job_timestamp_seconds` | gauge | when the finished job kept longest ended |
| `gusto_proxy_calls_total`, `gusto_proxy_errors_total`, `gusto_proxy_refused_total` | counter | model calls handled; those the model's side failed (no answer, or a 5xx); those a spending cap stopped. Absent when there is no proxy |
| `gusto_last_deploy_ok{release}`, `gusto_last_deploy_timestamp_seconds` | gauge | how the last deploy ended: 1, or 0 for a failure or a rollback. Present where a deploy writes `GUSTO_DEPLOY_RESULT`, as the playbook's autodeploy does |

Counters start again at zero when the server restarts, which Prometheus reads
as a reset.

## Alerts

`deploy/alerts/gusto.rules.yml` holds the rules, `gusto.rules.test.yml` tests them
with `promtool`, and `prometheus.example.yml` is a scrape of a server. Every
rule's `runbook` annotation points at its heading here.

### GustoDown

Prometheus could not scrape `/metrics` for two minutes. The server is down,
unreachable, or the token no longer works.

1. `curl -sS https://<host>/healthz`. If that answers and `/metrics` gives 401,
   the token was revoked or the credential changed: make a new one at `/admin/tokens`.
2. If nothing answers: `systemctl status gusto` and `journalctl -u gusto -n 100`.
   A server that will not start says why. Two refusals are deliberate: a
   change record that does not hold together, and data a newer release wrote
   ([UPGRADE.md](UPGRADE.md)).
3. If the last deploy is what broke it, see GustoDeployFailed.

### GustoNotReady

Fewer workers are running than `GUSTO_JOB_CONCURRENCY`. The queue takes jobs and
will not start all it should. `/readyz` gives 503 and says how many.

Restart the service: `systemctl restart gusto`. This ends running jobs, which
the server marks interrupted when it comes back. If it recurs, read the log for
the panic or error that took a worker, and report it.

### GustoQueueStuck

A queued job that no limit, no busy agent and no full pool holds back has waited
over five minutes. Workers are alive but not starting it, so something is failing
before a job starts: the log says `could not claim a job` when the jobs
directory cannot be written.

1. `df -h` on the jobs volume, and `journalctl -u gusto | grep "could not claim"`.
2. Free the disk (GustoDiskLow), or fix the directory's permissions.
3. `systemctl restart gusto` if the log shows nothing and the queue stays stuck.

### GustoRunnersSaturated

Every runner has been busy, with jobs waiting, for half an hour. Nothing is
broken. Look at Activity: is one person's work taking every runner, or is the
box too small for the team?

- One person: set `GUSTO_LIMIT_PERSON` ([LIMITS.md](LIMITS.md)).
- Too little capacity: raise `GUSTO_JOB_CONCURRENCY` if memory allows
  (`GUSTO_JOB_MEMORY` times the concurrency must fit), or add a machine.
- A job that hangs: its budget in seconds ends it; a very long budget is the
  usual cause.

### GustoJobsFailing

More than half the jobs that ended in the last hour failed, and at least five
did. A job fails when its check does or when it cannot run.

1. Open a few failed jobs from Activity. The page says which: the check, the
   harness's exit, or a refusal.
2. Every failure the same? Suspect what they share: a repository the GitHub App
   cannot reach ([REPOS.md](REPOS.md)), a model credential that expired, a
   runner image that changed.
3. `gusto doctor` on the server checks the machine can run a job.

### GustoDiskLow

Less than a tenth of the volume the jobs are on is free. Jobs then fail to
write, and the queue stalls.

1. `/admin/retention` shows what the jobs take and what a sweep would free. Set or
   shorten the policy ([RETENTION.md](RETENTION.md)), then sweep.
2. `gusto retention` lists a dry run from the command line.
3. Other things on the same volume take space too: check with `du`.

### GustoModelErrors

More than a fifth of the model calls in ten minutes failed on the model's side:
no answer, or a 5xx. At least ten calls were made.

1. Check the provider's status.
2. `journalctl -u gusto` and a failed job's page show the error the provider gave.
3. A credential that expired gives 4xx, which is not counted here: look at
   failed jobs if the calls are failing another way.
4. Refusals by a spending cap are counted apart, in `gusto_proxy_refused_total`.

### GustoDeployFailed

The last deploy failed, or rolled back. The server that is running is the
version from before it. The deploy's workflow run in GitHub says which step.

1. `journalctl -u 'gusto-deploy@*'` on the box has the play's output.
2. A smoke failure means the candidate could not run a job. A refusal from
   `gusto data check` means the data would not upgrade
   ([UPGRADE.md](UPGRADE.md)). A rollback means the new version did not answer
   `/readyz`.
3. Fix on a branch and merge: a merge to `main` deploys again, and a good deploy
   clears the alert.

### GustoRestartingOften

The server started three times in half an hour: a crash loop, or a deploy
that keeps restarting it. `journalctl -u gusto` shows the exit each time.
