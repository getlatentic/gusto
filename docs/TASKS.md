# The task index

A server with many tasks keeps one row for each task in a database. The task list reads those rows. It does not read every task directory, and it does not hold every task in memory. A second replica can read the same rows.

The index is not the record. The task's `state.json` stays the truth. The server writes the row each time it writes the file. If the index is deleted or drifts, the server and `gusto tasks reindex` make it again from the files.

This step does not move the queue. The queue, the limits and the quota still use the tasks the server holds in memory.

## What the list shows

Open it at `/jobs`. The page works as it did, and adds:
- A search box. It matches part of a task's id, brief, asker or repository. Case does not matter. `%` and `_` are themselves.
- A filter for the state, from the figures across the top: running, queued, passed and failed. An interrupted task is filed under failed.
- A filter for the agent type.
- A filter for the person. Only an administrator has it.
- An order: newest asked (the default), recently ended, or order of arrival.
- The total that matches, and a link to the next page.

While the index is being filled, a notice says how many tasks it has read ("The index is still being built, 3 of 12 tasks"). A short list at that time is not a short server.

A server with no index shows the list from memory, as before.

## Filters

| Filter | Query parameter | Notes |
| --- | --- | --- |
| Search | `q` | Id, brief, asker or repository |
| State | `state` | `queued`, `running`, `passed`, `failed` or `interrupted`, or several separated by commas. On the page, `failed` includes `interrupted` |
| Person | `person` | Who asked. Case does not matter |
| Agent type | `agent_type` | Exact |
| Team | `team` | Exact. The API only |
| Repository | `repo` | Exact. The API only |
| Instance, thread | `instance`, `thread` | Exact. The API only |
| From, to | `from`, `to` | When the task was asked for. An RFC 3339 time, or a date. The API only |

`GET /api/jobs` takes the same parameters. [PROTOCOL.md](PROTOCOL.md) has its answer.

## Order and pages

`sort` is `queued` (the default), `ended` or `seq`. Each is from the greatest: the task asked for last, the task that ended last, or the task that arrived last. A task that has not ended comes last in `ended`. Tasks of equal rank come in the order they arrived.

Pages continue from the last task of the page before, not from a count. A task that arrives or goes while someone reads does not move the next page. `limit` is 1 to 1000, and 50 when it is not given. `after` is the `next` of the last answer. It is valid only with the `sort` that made it. Any other cursor is answered 400.

The page links to the next page and back to the first. It does not link to the page before.

## Who sees what

- An administrator sees every task.
- Anyone else sees only the tasks they asked for. The server sets the person to the signed-in person on every route. A `person` parameter cannot widen it, and neither can a search or any other filter.
- A task that nobody asked for, or that the shared sign-in asked for, is an administrator's alone. This is the rule every other page uses.
- A person's token is never an administrator's.

## How the index is kept current

- Each time the server writes a task's `state.json` (a task is queued, claimed, stopped or finished), it queues a copy of the task's row. One goroutine writes the queue to the database in order. Removing a task queues the removal of its row.
- A slow or broken database never holds up a task. The queue is bounded. A change that finds it full, or that the database refuses, is counted and logged (once a minute at most), and left for the next reconcile.
- The reconcile compares the task directories with the rows. It adds a row that is missing, replaces a row that differs from the task in any field, and removes the row of a task whose directory is gone. The server runs it in the background when it starts, and again after a change was not written. A run that fails is tried again after 30 seconds.
- A task that changes while a reconcile runs is not set back. The server writes its own copy of a task it holds, and writes it behind the changes already queued.
- A task whose `state.json` cannot be read just then, because it is being written, keeps its row.

## Commands

```bash
gusto tasks list -state failed,interrupted -month 2026-10
gusto tasks list -person ada@acme.com -search login -json
gusto tasks reindex
gusto tasks reindex -from-scratch
```

`gusto tasks reindex` reads the files and changes nothing in them. It can run while a server runs. With `-from-scratch` it removes every row first and says how many. `gusto doctor` has a "task index" check, and `gusto data status` has a line with the schema version and the number of rows. Neither changes the database.

## The database

The task index and the [usage index](USAGE.md) use the same setting.

| `GUSTO_DATABASE_URL` | The task index is |
| --- | --- |
| empty | A SQLite file, `tasks.db`, in the directory that holds `GUSTO_JOBS` |
| `postgres://...` | In Postgres, in the same schema as the usage index |

A `search_path` parameter in the Postgres URL gives each organization its own schema. The tables are `tasks` and `tasks_schema_migrations`. The server upgrades the schema when it starts, and refuses a database that a newer gusto made. It uses only features of Postgres 13 and later. Postgres 14 was tested.

## Size

These times were measured on one laptop with 100,000 tasks in the index, on SQLite and on Postgres 14 on the same machine. Each is the best of three runs.

| Read | SQLite | Postgres |
| --- | --- | --- |
| First page of the list | 0.2 ms | 0.2 ms |
| A person's first page | 0.2 ms | 0.3 ms |
| A person's failed tasks | 3.7 ms | 0.7 ms |
| Search for a rare phrase | 59 ms | 13 ms |
| Search that matches nothing | 117 ms | 29 ms |
| Count by state | 13 ms | 19 ms |
| Count of a person's tasks | 4 ms | 1 ms |
| Agent types | 10 ms | 28 ms |

A search reads every row when few rows match, so it is the one that grows with the number of tasks. It stays under 300 ms at this size.

| Reconcile 100,000 task directories | SQLite | Postgres |
| --- | --- | --- |
| Into an empty index | 9.1 s | 5.0 s |
| Against a current index | 1.5 s | 1.6 s |

Reading the 100,000 `state.json` files takes 1.6 s of that. `gusto serve` read them one at a time before, which took 14.6 s in one measurement. It reads 16 at a time now, and the same read took 2.5 s. `go test ./internal/taskindex -run TestPerf` and `GUSTO_TEST_BIG=1 go test ./internal/jobs -run TestReconcileOnAHundred` repeat the measurements.
