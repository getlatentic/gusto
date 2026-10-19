# The gusto protocol

Every client uses the same calls in the same shapes: the desktop app, the helper and your scripts. Only sign-in differs by place. The calls are one table in [`internal/web/protocol.go`](../internal/web/protocol.go). `GET /api/protocol` reads it back (`{"protocol":1,"calls":[...]}`), and a test fails when this document and the table disagree. `ProtocolVersion` is 1. A change that breaks a client raises it.

## Where the server is

| Where | Address | How a client signs in |
| --- | --- | --- |
| Your computer (`gusto local`) | `http://127.0.0.1:<port>`, loopback only | A link that works once for five minutes (`gusto local link`) opens a session. Another computer of yours uses the device flow |
| Another computer you own | its address, behind TLS | The shared password opens a session. With no identity provider you are the one owner and approve a computer by the device flow |
| An organisation's | its address, behind TLS | Single sign-on opens a session. A person approves a computer by the device flow. A program carries a token with a scope |

The desktop app and the helper refuse a plain `http` address unless it is this computer's own.
## Signing in

A session is a cookie, for the pages. A token is a bearer, for programs: `Authorization: Bearer <token>`.

A computer gets a person's token by the device flow. The server needs `GUSTO_API_TOKENS`.
1. `POST /api/device` with `{"label": "my laptop"}` returns `user_code`, `device_code`, the address to visit, `expires_in` and `interval`.
2. The person opens `/device`, signs in and types the code.
3. The computer polls `POST /api/device/token` with `{"device_code": ...}`. The answer is `{"error": "authorization_pending"}` until the person approves, then once `{"access_token": "gusto_...", "person": "...", "token_type": "bearer"}`.

The token acts as that person, on their own work. The person ends it from `/device`, or the computer ends it with `DELETE /api/me`.

## The calls

"A person" is a device-flow token. "Token: x" is an organisation token with scope x. Somebody else's work is answered as missing.

| Call | Who | What it does |
| --- | --- | --- |
| `POST /api/device` | nobody | Ask to sign in. Returns a code for the person to approve |
| `POST /api/device/token` | nobody | Ask whether the person approved. Returns a token once they have |
| `GET /api/me` | a person | Who this token acts as |
| `DELETE /api/me` | a person | Sign this computer out: its token ends |
| `GET /api/instances` | a person | The person's instances |
| `POST /api/instances` | a person | Start an instance of an agent type, in a folder of its own |
| `GET /api/engines` | a person | The agent types this server can start |
| `GET /api/instances/{id}` | a person | One instance |
| `GET /api/instances/{id}/threads` | a person | An instance's threads |
| `POST /api/instances/{id}/threads` | a person | Open a thread |
| `GET /api/instances/{id}/threads/{thread}` | a person | A thread: what was said and what came back |
| `POST /api/instances/{id}/threads/{thread}/say` | a person | Say something, which starts a turn. An Idempotency-Key makes a retry the same turn |
| `GET /api/jobs` | a person | The person's tasks, newest first, narrowed by `q`, `state` and the other filters, a page at a time (docs/TASKS.md) |
| `GET /api/jobs/{id}` | a person | One turn: its steps, reply and state |
| `GET /api/jobs/{id}/follow` | a person | Follow one turn (a stream) |
| `POST /api/jobs/{id}/stop` | a person | Stop a turn that is running |
| `GET /api/events` | a person | Every change to the person's turns, with catch-up from ?since (a stream) |
| `GET /api/instances/{id}/files` | a person | The files in an instance's folder |
| `GET /api/instances/{id}/file` | a person | One file's bytes |
| `PUT /api/instances/{id}/file` | a person | Write one file |
| `DELETE /api/instances/{id}/file` | a person | Remove one file |
| `POST /api/instances/{id}/hashes` | a person | The hashes of the files named, so a client sends only what differs |
| `POST /api/jobs` | token: submit | Send a task directory to be run |
| `GET /api/spend` | token: spend | The spend report |
| `GET /api/usage/sessions` | token: spend, or a person | The model calls added up by session, the latest first (docs/USAGE.md) |
| `GET /api/usage/calls` | token: spend, or a person | One session's calls, the latest first, from `?session=` |
| `GET /api/usage/totals` | token: spend, or a person | The calls that match, added up |
| `GET /api/usage/facets` | token: spend, or a person | The people, agents, agent types, providers and models that calls have |
| `GET /api/usage.csv` | token: spend, or a person | Every call that matches, as CSV (a stream) |
| `GET /api/changes` | token: audit | The record of what changed on the platform |
| `GET /api/protocol` | nobody | The protocol version and this table, as JSON |

## Reading usage

The five `/api/usage` calls read the index of model calls. A person's token reads that person's own calls. A token with the `spend` scope reads all of them. The calls also take a signed-in session, which is how the **Download CSV** link on `/usage` works. Every filter narrows the lists, the totals, the CSV and a session's calls. The lists take `limit` (up to 1000) and `after`, and the sessions take `sort` (`last`, `cost`, `calls` or `tokens`, from the greatest). The filters and the answers are in [USAGE.md](USAGE.md).

## Listing tasks

`GET /api/jobs` lists the tasks of the person whose token it is, from the index of tasks, the newest asked first. A person is never shown another person's task: a `person` parameter cannot widen the list. A server with no index answers 404.

| Parameter | Meaning |
| --- | --- |
| `q` | Part of a task's id, brief, asker or repository. Case does not matter, and `%` and `_` are themselves |
| `state` | `queued`, `running`, `passed`, `failed` or `interrupted`, or several separated by commas |
| `agent_type`, `team`, `repo`, `instance`, `thread` | Exact |
| `from`, `to` | When the task was asked for. An RFC 3339 time, or a date. `to` is not included when it is a time, and is included when it is a date |
| `sort` | `queued` (the default), `ended` or `seq`, from the greatest |
| `limit`, `after` | 1 to 1000 (50 by default), and the `next` of the last answer |

The answer is `{"tasks":[...],"counts":{"passed":12,...},"next":"..."}`. `counts` is how many tasks match, by state, over all pages. `next` is missing on the last page. A cursor made for one `sort` is answered 400 when it is used with another. An answer given while the index is still being filled has a `Task-Index-Building: 3 of 12 tasks` header.

```bash
curl -H "Authorization: Bearer $TOKEN" "$GUSTO/api/jobs?state=failed,interrupted&q=login&limit=20"
```

## A turn

Saying something in a thread starts a turn, which is a task. These requests ran against a local gusto with a stand-in model, on an instance started with `POST /api/instances`.

```bash
curl -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: say-1" -X POST \
  $GUSTO/api/instances/20261013-0924-pi/threads/1/say -d '{"said":"What is 17 times 23? Answer with just the number."}'
# {"already":false,"job":"20261013-0924-what-is-17-times-23-2","state":"queued"}
# Repeating the Idempotency-Key answers {"already":true,...} and starts nothing.
# The `job` field and the `/api/jobs` paths name the task.

curl -N -H "Authorization: Bearer $TOKEN" $GUSTO/api/jobs/20261013-0924-what-is-17-times-23-2/follow
# event: ended, data: {"job":{"id":"...","state":"passed",...},"steps":[],"reply":"391","changed":[]}
```

## The streams

Two calls are server-sent events (`text/event-stream`).
- `/api/jobs/{id}/follow` sends `step` for each thing the agent does, `reply` with the text so far, and `ended` with the whole turn. A finished turn is told at once.
- `/api/events` follows all the person's turns. A connection first hears the state of every turn that has not ended and of every turn that ended after `?since=<unix seconds>`, then `ready`, then each change. A comment line is the keep-alive.

## Known gaps
- A stream cannot resume at an event id. `/api/events` catches up by time, and `follow` replays a turn from its start.
- A turn whose model call failed can end `passed` with an empty `reply` (getlatentic/planning#676).
