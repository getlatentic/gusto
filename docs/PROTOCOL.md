# The haas protocol

How a client talks to a haas server. It is one protocol: the same calls, in the same shapes, whether the server is on your own
computer, on another computer you own, or an organisation's. The browser, the desktop app, the helper and your own scripts are all clients
of it. What differs from place to place is only how a client signs in.

The protocol is in the code, in one table: [`internal/web/protocol.go`](../internal/web/protocol.go). The routes are made from that
table, `GET /api/protocol` reads it back from a running server, and a test fails if this document and the table disagree. The version is
`ProtocolVersion` (now 1), and a change that breaks a client raises it.

```bash
curl http://127.0.0.1:7140/api/protocol     # {"protocol":1,"calls":[...]}
```

## Where the server is

| Where | Address | How a client signs in |
| --- | --- | --- |
| Your own computer (`haas local`) | `http://127.0.0.1:<port>`, the loopback only | a link that works once for five minutes (`haas local link`) opens a session; a computer of yours signs in by the device flow |
| Another computer you own | its address, behind TLS | the shared password opens a session, and with no identity provider it is the one owner and approves a computer by the device flow |
| An organisation's | its address, behind TLS | single sign-on opens a session, and a person approves a computer by the device flow; an organisation's program carries a token with a scope |

The desktop app and the helper refuse a plain `http` address unless it is this computer's own. Everything below is the same in all three.

## Signing in

A **session** is a cookie, for the pages. A **token** is a bearer, for programs: `Authorization: Bearer <token>`.

A computer gets a token for a person by the device flow, which is why it needs `HAAS_API_TOKENS` on the server:

1. `POST /api/device` with `{"label": "my laptop"}` answers a `user_code`, a `device_code`, the address to visit, and how long and how often (`expires_in`, `interval`).
2. The person opens `/device`, signs in as they usually do, and types the code.
3. The computer polls `POST /api/device/token` with `{"device_code": ...}`. It is answered `{"error": "authorization_pending"}` until the person approves, then once `{"access_token": "haas_...", "person": "...", "token_type": "bearer"}`.

The token acts as that person, on that person's own work, and a person can end it from `/device`. `DELETE /api/me` ends it from the computer.

## The calls

Who may call is **nobody** (signing in), **a person** (a token from the device flow), or an organisation's **token** with the scope named.
Somebody else's work is answered as missing, as the pages answer it.

| Call | Who | What it does |
| --- | --- | --- |
| `POST /api/device` | nobody | a computer asks to sign in, and is given a code for its person to approve |
| `POST /api/device/token` | nobody | the computer asks whether its person approved, and is given a token the once they have |
| `GET /api/me` | a person | who this token acts as |
| `DELETE /api/me` | a person | sign this computer out: its token ends |
| `GET /api/instances` | a person | the person's instances |
| `POST /api/instances` | a person | start an instance of an engine, in a folder of its own |
| `GET /api/engines` | a person | the engines this server can start |
| `GET /api/instances/{id}` | a person | one instance |
| `GET /api/instances/{id}/threads` | a person | an instance's threads |
| `POST /api/instances/{id}/threads` | a person | open a thread |
| `GET /api/instances/{id}/threads/{thread}` | a person | a thread: what was said and what came back |
| `POST /api/instances/{id}/threads/{thread}/say` | a person | say something in a thread, which starts a turn; an Idempotency-Key makes a retry the same turn |
| `GET /api/jobs/{id}` | a person | one turn: its steps, its reply and its state |
| `GET /api/jobs/{id}/follow` | a person | follow one turn: each step, the reply as it grows, and the end (a stream) |
| `POST /api/jobs/{id}/stop` | a person | stop a turn that is running |
| `GET /api/events` | a person | every change to the person's turns, with catch-up from ?since (a stream) |
| `GET /api/instances/{id}/files` | a person | the files in an instance's folder |
| `GET /api/instances/{id}/file` | a person | one file's bytes |
| `PUT /api/instances/{id}/file` | a person | write one file |
| `DELETE /api/instances/{id}/file` | a person | remove one file |
| `POST /api/instances/{id}/hashes` | a person | the hashes of the files named, so a client sends only what differs |
| `POST /api/jobs` | token: submit | send a job directory to be run, as an organisation's program does |
| `GET /api/spend` | token: spend | the spend report |
| `GET /api/changes` | token: audit | the record of what changed on the platform |
| `GET /api/protocol` | nobody | the version of this protocol and this table, as JSON |

## A turn

A person's work is four steps: be signed in, have an instance, say something in one of its threads, and follow what happens. Saying
something starts a **turn**, which is a job. Every request below is from a run of a haas on this computer with a stand-in model.

```bash
# sign in (the person approves the code at /device, then the second call answers the token)
curl -X POST $HAAS/api/device -d '{"label":"my laptop"}'
# {"device_code":"...","expires_in":600,"interval":3,"user_code":"XKDR-ZBNM","verification_uri":"http://127.0.0.1:7140/device"}
curl -X POST $HAAS/api/device/token -d '{"device_code":"..."}'
# {"access_token":"haas_...","person":"dev","token_type":"bearer"}      before approval: {"error":"authorization_pending"}

curl -H "Authorization: Bearer $TOKEN" $HAAS/api/me
# {"person":"dev","role":"runner"}

# an instance of an engine, which comes with its first thread
curl -H "Authorization: Bearer $TOKEN" -X POST $HAAS/api/instances -d '{"harness":"pi","model":"local/fake-small"}'
# {"instance":{"id":"20261013-0924-pi","harness":"pi","model":"local/fake-small","who":"dev","startedAt":1791883217},"thread":{"id":"1",...}}

# say something: the turn is queued and its id is given back
curl -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: say-1" -X POST \
  $HAAS/api/instances/20261013-0924-pi/threads/1/say -d '{"said":"What is 17 times 23? Answer with just the number."}'
# {"already":false,"job":"20261013-0924-what-is-17-times-23-2","state":"queued"}
# the same Idempotency-Key again answers {"already":true,...} and starts nothing

# follow it
curl -N -H "Authorization: Bearer $TOKEN" $HAAS/api/jobs/20261013-0924-what-is-17-times-23-2/follow
# event: ended
# data: {"job":{"id":"...","state":"passed",...},"steps":[],"reply":"391","changed":[]}
```

## The streams

Two calls are streams of server-sent events (`text/event-stream`), which a client reads for as long as it likes.

- **`/api/jobs/{id}/follow`** follows one turn. It sends `step` for each thing the agent does, `reply` with the text so far whenever it changes, and `ended`
  with the whole turn when it is over. A turn that is already over is told at once, so following one is always safe.
- **`/api/events`** follows all of the person's turns, so a tray or an editor learns when any of them starts, waits or ends. A connection first hears the
  state of every turn that has not ended, and of every turn that ended after `?since=<unix seconds>`, and then `ready`; after that it hears each change, with a
  comment line as a keep-alive. A client that was away passes the time it last heard from the server, and so catches up.

`POST /api/jobs/{id}/stop` stops a turn that is running, and answers whether it was.

## Next for the protocol

- A stream cannot resume at an exact event: `/api/events` catches up by time, not by an event's id, and `follow` replays a turn from its start.
- The server finds changes by looking at the job every 300 milliseconds, so a stream is not pushed the moment something happens.
- A turn whose model call failed will say so: today it can end `passed` with an empty `reply`, because the engine itself exited cleanly (getlatentic/planning#676).
- The pages are HTML, and the desktop app is a window onto them. They are not a client of this protocol.
