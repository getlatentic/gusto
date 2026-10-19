# Limits on what one person or team runs at once

`GUSTO_JOB_CONCURRENCY` sets how many tasks the server runs at once. Two settings stop one person or team from taking all of them.

| Setting | Meaning | Playbook | Chart |
| --- | --- | --- | --- |
| `GUSTO_LIMIT_PERSON` | the most tasks one person runs at once | `limit_person` | `jobs.limitPerson` |
| `GUSTO_LIMIT_TEAM` | the most tasks one team runs at once | `limit_team` | `jobs.limitTeam` |

Zero or unset is no limit. Compose takes the two variables in `.env`.

A person is the signed-in address, the shared sign-in (one person), or `token:<name>` for a task an API token sent. A team is the `team` the task names, which is also what spending groups by. A task with no team is held by no team limit.

A limit is a hard cap. With two runners and a person limit of one, a second runner stays idle when nobody else waits. Set the person limit to the share of runners one person should hold. Leave it unset on a server with one user.

Comparisons start one task per agent. A person limit of one makes a comparison run its agents one after another.

## A task over its limit

The task stays queued. It starts when one of that person's or team's tasks ends. Otherwise the order is first come, first served.

The task's page and its Activity line say what holds it, and where it stands in the queue. The Activity page's *waiting* filter is the queue.

| Message | Cause |
| --- | --- |
| Amy already has 1 running, the most one person may run at once | the person limit |
| The team platform already has 2 running, the most a team may run at once | the team limit |
| Every runner is busy: 2 of 2 | every worker is on a task |
| Waiting for this agent to finish its last message | a turn of the same agent is running |

## A quota for the whole server

A hosted organization's server runs under a quota that its gusto cloud writes ([CLOUD.md](CLOUD.md)). `GUSTO_QUOTA` names the file:

```json
{"jobs": 3, "concurrent": 1, "note": "Confirm the admin's email address to raise this limit."}
```

| Field | Meaning |
| --- | --- |
| `jobs` | how many tasks may start in all, or in a calendar month (UTC) when `monthly` is true |
| `concurrent` | how many may run at once, whoever asked |
| `note` | a sentence added to the refusal, saying how to lift the limit |

Zero is none.

- A task past `jobs` is refused when asked for. The pages show the reason and the API answers 429.
- A task past `concurrent` stays queued and says "This organization may run 1 at a time and has 1 running".
- The server rereads the file each second. A lifted limit applies to the next task without a restart.
- A server started with `GUSTO_QUOTA` and an unreadable file does not start.
- A server that loses the file while running refuses every new task and runs one at a time. It does not fall back to no limit.
