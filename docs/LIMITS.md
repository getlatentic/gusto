# Limits on what one person or team runs at once

`HAAS_JOB_CONCURRENCY` says how many jobs the server runs at once. Two more
settings stop one person or team from taking all of them.

| Setting | Meaning |
| --- | --- |
| `HAAS_LIMIT_PERSON` | the most jobs one person runs at once |
| `HAAS_LIMIT_TEAM` | the most jobs one team runs at once |

Zero, or nothing, is no limit. The playbook has `limit_person` and `limit_team`,
compose has the two variables in `.env`, and the chart has `jobs.limitPerson`
and `jobs.limitTeam`.

## Who counts

A person is who asked for the job: the signed-in address, the shared sign-in as
one person, or `token:<name>` for a job an API token sent. A team is the
`team` the job names, which is also what spending groups by. A job that names
no team is held by no team limit.

## What a job over its limit does

It stays queued. It starts when one of that person's or team's jobs ends, and a
job that belongs to nobody at their limit starts in its place. Order is
otherwise first come, first served.

The job's page, and its line in Activity, says which thing holds it:

| Says | Because |
| --- | --- |
| Amy already has 1 running, the most one person may run at once | the person limit |
| The team platform already has 2 running, the most a team may run at once | the team limit |
| Every runner is busy: 2 of 2 | every worker is on a job |
| Waiting for this agent to finish its last message | a turn of the same agent is running |

Each also says where the job stands in the line, counting every queued job from
the front. The Activity page's *waiting* filter is the queue.

## Choosing

A limit is a hard cap. With two runners and a person limit of one, one person's
job leaves the second runner idle when nobody else is waiting. That is the cost
of a cap that never varies. Set the person limit to the share of runners one
person is meant to hold, and leave it unset on a server with one user.

Comparisons start one job for each engine, so a person limit of one makes a
comparison run its engines one after another.

## A quota for the whole server

A hosted organisation's server runs under a quota that its front door writes
(`docs/CLOUD.md`). `HAAS_QUOTA` names the file:

```json
{"jobs": 3, "concurrent": 1, "note": "Confirm the admin's email address to raise this limit."}
```

| Field | Meaning |
| --- | --- |
| `jobs` | how many jobs may be started, in all, or in a calendar month (UTC) when `monthly` is true |
| `concurrent` | how many may run at once, whoever asked for them |
| `note` | a sentence added to the refusal: how the limit is lifted |

Zero is none. A job past `jobs` is refused when it is asked for: the pages show
the reason, and the API answers 429 with it. A job past `concurrent` stays queued
and says "This organisation may run 1 at a time and has 1 running".

The file is read again each second, so a limit that is lifted applies to the next
job without a restart. A server started with `HAAS_QUOTA` and a file it cannot
read does not start. One that loses the file while it runs refuses every new job
and lets one run at a time, and does not fall back to no limit.

