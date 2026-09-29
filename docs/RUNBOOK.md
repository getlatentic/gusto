# Runbook

What to watch on a haas server, and what to do when an alert fires.

## Health endpoints

| Path | Answers | Use it for |
| --- | --- | --- |
| `/healthz` | `ok` while the process runs | a liveness probe. It never depends on the workers or the disk |
| `/readyz` | `ready`, or 503 saying what is wrong: fewer workers running than `HAAS_JOB_CONCURRENCY`, or the jobs directory cannot be written | a readiness probe, and the check a deploy makes before it keeps a new version |
| `/metrics` | the Prometheus text format | scraping |

`/metrics` needs `Authorization: Bearer`. On a server with API tokens
([TOKENS.md](TOKENS.md)), make a token that may **read metrics** at `/tokens`.
On a server without, the shared password is the credential.

## Metrics

| Metric | Type | Says |
| --- | --- | --- |
| `haas_build_info{release}` | gauge | the release this server is |
| `haas_started_timestamp_seconds` | gauge | when it started |
| `haas_jobs{state}` | gauge | jobs held, by `queued`, `running`, `passed`, `failed`, `interrupted` |
| `haas_queue_depth` | gauge | jobs waiting to start |
| `haas_queue_oldest_wait_seconds` | gauge | how long the longest-waiting job has waited |
| `haas_queue_stuck_seconds` | gauge | how long the longest-waiting job has waited that **nothing holds back**: no limit, no busy agent, a runner free. Near zero while the queue moves |
| `haas_jobs_running` | gauge | jobs running now |
| `haas_runner_slots` | gauge | how many jobs run at once |
| `haas_runner_workers_alive` | gauge | workers running. Fewer than the slots means the queue is broken |
| `haas_jobs_finished_total{state}` | counter | jobs that ended since the server started |
| `haas_last_job_started_timestamp_seconds`, `haas_last_job_finished_timestamp_seconds` | gauge | since the server started; 0 before the first |
| `haas_disk_free_bytes{store="jobs"}`, `haas_disk_total_bytes{store="jobs"}` | gauge | the volume the jobs are on |
| `haas_jobs_disk_bytes`, `haas_jobs_workspace_bytes` | gauge | what the jobs take, and the workspaces among them. Measured at most once a minute |
| `haas_oldest_finished_job_timestamp_seconds` | gauge | when the finished job kept longest ended |
| `haas_proxy_calls_total`, `haas_proxy_errors_total`, `haas_proxy_refused_total` | counter | model calls handled; those the model's side failed (no answer, or a 5xx); those a spending cap stopped. Absent when there is no proxy |
| `haas_last_deploy_ok{release}`, `haas_last_deploy_timestamp_seconds` | gauge | how the last deploy ended: 1, or 0 for a failure or a rollback. Present where a deploy writes `HAAS_DEPLOY_RESULT`, as the playbook's autodeploy does |

Counters start again at zero when the server restarts, which Prometheus reads
as a reset.

## Alerts

`deploy/alerts/haas.rules.yml` holds the rules, `haas.rules.test.yml` tests them
with `promtool`, and `prometheus.example.yml` is a scrape of a server. Every
rule's `runbook` annotation points at its heading here.

### HaasDown

Prometheus could not scrape `/metrics` for two minutes. The server is down,
unreachable, or the token no longer works.

1. `curl -sS https://<host>/healthz`. If that answers and `/metrics` gives 401,
   the token was revoked or the credential changed: make a new one at `/tokens`.
2. If nothing answers: `systemctl status haas` and `journalctl -u haas -n 100`.
   A server that will not start says why. Two refusals are deliberate: a
   change record that does not hold together, and data a newer release wrote
   ([UPGRADE.md](UPGRADE.md)).
3. If the last deploy is what broke it, see HaasDeployFailed.

### HaasNotReady

Fewer workers are running than `HAAS_JOB_CONCURRENCY`. The queue takes jobs and
will not start all it should. `/readyz` gives 503 and says how many.

Restart the service: `systemctl restart haas`. This ends running jobs, which
the server marks interrupted when it comes back. If it recurs, read the log for
the panic or error that took a worker, and report it.

### HaasQueueStuck

A queued job that no limit, no busy agent and no full pool holds back has waited
over five minutes. Workers are alive but not starting it, so something is failing
before a job starts: the log says `could not claim a job` when the jobs
directory cannot be written.

1. `df -h` on the jobs volume, and `journalctl -u haas | grep "could not claim"`.
2. Free the disk (HaasDiskLow), or fix the directory's permissions.
3. `systemctl restart haas` if the log shows nothing and the queue stays stuck.

### HaasRunnersSaturated

Every runner has been busy, with jobs waiting, for half an hour. Nothing is
broken. Look at Activity: is one person's work taking every runner, or is the
box too small for the team?

- One person: set `HAAS_LIMIT_PERSON` ([LIMITS.md](LIMITS.md)).
- Too little capacity: raise `HAAS_JOB_CONCURRENCY` if memory allows
  (`HAAS_JOB_MEMORY` times the concurrency must fit), or add a machine.
- A job that hangs: its budget in seconds ends it; a very long budget is the
  usual cause.

### HaasJobsFailing

More than half the jobs that ended in the last hour failed, and at least five
did. A job fails when its check does or when it cannot run.

1. Open a few failed jobs from Activity. The page says which: the check, the
   harness's exit, or a refusal.
2. Every failure the same? Suspect what they share: a repository the GitHub App
   cannot reach ([REPOS.md](REPOS.md)), a model credential that expired, a
   runner image that changed.
3. `haas doctor` on the server checks the machine can run a job.

### HaasDiskLow

Less than a tenth of the volume the jobs are on is free. Jobs then fail to
write, and the queue stalls.

1. `/retention` shows what the jobs take and what a sweep would free. Set or
   shorten the policy ([RETENTION.md](RETENTION.md)), then sweep.
2. `haas retention` lists a dry run from the command line.
3. Other things on the same volume take space too: check with `du`.

### HaasModelErrors

More than a fifth of the model calls in ten minutes failed on the model's side:
no answer, or a 5xx. At least ten calls were made.

1. Check the provider's status.
2. `journalctl -u haas` and a failed job's page show the error the provider gave.
3. A credential that expired gives 4xx, which is not counted here: look at
   failed jobs if the calls are failing another way.
4. Refusals by a spending cap are counted apart, in `haas_proxy_refused_total`.

### HaasDeployFailed

The last deploy failed, or rolled back. The server that is running is the
version from before it. The deploy's workflow run in GitHub says which step.

1. `journalctl -u 'haas-deploy@*'` on the box has the play's output.
2. A smoke failure means the candidate could not run a job. A refusal from
   `haas data check` means the data would not upgrade
   ([UPGRADE.md](UPGRADE.md)). A rollback means the new version did not answer
   `/readyz`.
3. Fix on a branch and merge: a merge to `main` deploys again, and a good deploy
   clears the alert.

### HaasRestartingOften

The server started three times in half an hour: a crash loop, or a deploy
that keeps restarting it. `journalctl -u haas` shows the exit each time.
