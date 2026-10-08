# API tokens

A program that calls the server carries a token, not the shared password. A
token may do only what it was given, is named in every record it makes, and stops
working the moment an admin revokes it. Nothing else that uses the server changes
when one token ends.

## Turn it on

Set `HAAS_API_TOKENS` to the file the tokens are kept in. The server makes the
file when the first token is made.

| Install | Setting |
| --- | --- |
| Compose | `HAAS_API_TOKENS=/var/lib/haas/tokens/tokens.json` in `.env` |
| Chart | `auth.apiTokens: true` |
| Playbook | `api_tokens: true` |

With tokens on, the API refuses the shared password. Without them it takes the
shared password, as before. The pages keep their own sign-in either way.

## Make one

An admin opens **Tokens** in the admin console and gives a name in lowercase letters, digits and
dashes, and what the token may do.

| It may | What that opens |
| --- | --- |
| send jobs | `POST /api/jobs` |
| read spending | `GET /api/spend` |
| read the change record | `GET /api/changes` |
| read metrics | `GET /metrics` |

The page shows the secret once, in the response to making it. The server keeps
only a hash of it, so nobody can show it again: make another and revoke the old
one.

```bash
export HAAS_SERVER=https://haas.example.com
export HAAS_TOKEN=haas_3fa1c2d0_...
haas submit ./job
```

`haas submit` sends `HAAS_TOKEN` when it is set, and `HAAS_PASSWORD` otherwise.
Any other caller sends `Authorization: Bearer <token>`.

## A token for a person

A person's own computer gets a token that acts as that person, by the device flow.
It needs the token file above, and it is not made at `/admin/tokens`.

1. The computer asks the server for a code: `POST /api/device`, with `{"label":"Laptop"}`.
   The answer has a `device_code` for the computer, a `user_code` for the person,
   and the address to visit.
2. The person signs in as usual, opens `/device`, types the user code and approves it.
   The shared sign-in cannot approve one, since it is nobody in particular.
3. The computer asks `POST /api/device/token` with its `device_code` every few seconds.
   It is told `authorization_pending` until the person approves, and then given the
   token once. A code lasts ten minutes.

`haas-helper login` (or `haas login`) does these steps on a person's computer: it shows the address and the
code, opens the browser, waits for the approval, and keeps the token in the
operating system's keychain (the Keychain on macOS, the Credential Manager on
Windows, the Secret Service on Linux). With no keychain it keeps the token in a file
in the person's own settings folder that only they can read, and says so;
`HAAS_VAULT=file` asks for the file alone. `whoami` asks the server who the
token acts as, and `logout` ends the token on the server and forgets it.

```bash
haas-helper login -server https://haas.example.com
haas-helper whoami -server https://haas.example.com
haas-helper logout -server https://haas.example.com
```

The token has one scope, `work`. It can do nothing an organisation's token does:
it cannot send a job, read spending, the change record or metrics. `GET /api/me`
says who it acts as. The person sees their own tokens at `/device` and ends any of
them there, and an admin sees them all at `/admin/tokens`. Making one and ending one
are in the record of what changed on the platform. A server with no token file has
no device flow, and its pages answer 404.

With that token a program reads the person's own work as JSON. Every route answers
about the person's own agents, and somebody else's is answered as one that is not
there.

| Route | Answers |
| --- | --- |
| `GET /api/instances` | the person's agents |
| `GET /api/instances/{id}` | one agent and its threads |
| `GET /api/instances/{id}/threads` | its threads |
| `GET /api/instances/{id}/threads/{thread}` | one thread, and the turns said in it as the jobs they were |
| `POST /api/instances/{id}/threads` | opens a thread: `{"title": "..."}`, answered `201` with the thread |
| `POST /api/instances/{id}/threads/{thread}/say` | says something in it: `{"said": "..."}`, answered `202` with the job, the turn the harness will take |
| `POST /api/instances` | starts an agent: `{"harness": "...", "model": "...", "repo": "...", "setup": "...", "title": "..."}`, with only the harness required. It follows the page's rules: only an agent this server can run now, only a model the person may use, and a repository from where a repository may come. Answered `201` with the agent and its first thread |
| `GET /api/engines` | the agents this server can start, whether each can run now and why not, and the models this person may use with each |
| `GET /api/jobs/{id}` | one turn: how it stands, the steps the agent took in words (`Read main.go`), what it said, and the files it changed |
| `GET /api/jobs/{id}/follow` | the same turn as server-sent events while it goes: `step` for each step, `reply` with what has been said so far when that changes, and `ended` with the whole turn. A turn that is over is told at once |
| `POST /api/jobs/{id}/stop` | stops a turn: a queued one never runs, and a running one is told to stop and ends `interrupted` a moment later. Answered `200` with the job and whether it was running; `409` when it is already over ([STATES.md](STATES.md#stopping-a-job)) |
| `GET /api/instances/{id}/files` | the regular files of its folder: path, size and time changed. Links and git's own directory are left out, and a list is cut at 50,000 files, with `cut` set |
| `PUT /api/instances/{id}/file?path=` | puts a file in the agent's folder, whole or not at all, folders made as needed. `X-If-Hash` says what the writer saw (a hash, or `absent`) and the server answers `412` with the hash it is when it is anything else; `X-Mode: 755` keeps the executable bit. Files over 64 MiB are `413`. A path with `..`, a backslash, a `.git` part, or a link on the way is `400` |
| `DELETE /api/instances/{id}/file?path=` | deletes one, under the same `X-If-Hash` check; answered `204` |
| `POST /api/instances/{id}/hashes` | `{"paths": [...]}`, at most 5000, answered with the sha256 of each that is a plain file, so a helper reads only what changed |
| `GET /api/instances/{id}/file?path=` | one file whole, with ranges. A path that leaves the folder, by a link or otherwise, is not found |

`GET /api/events` is a stream of server-sent events about the person's own jobs, for a
tray or a notification. A connection first hears the state of every job that has not
ended, and of every job that ended after `?since=<unix seconds>` when the client names
one, which is how it catches up after it was away. An event `ready` marks the end of
that. After it, each job's change is one event:

| Event | When |
| --- | --- |
| `turn.queued` | the job waits |
| `turn.started` | it runs |
| `turn.finished` | it passed |
| `turn.failed` | it failed |
| `turn.interrupted` | it was stopped |

Each carries `{"job","instance","thread","brief","state","at"}`. A comment line every
twenty seconds keeps the connection open through a proxy. A job that waits for an
approval has no event yet, because the product has no approval to wait for.

**A send made twice is one send.** `POST /api/jobs` and a thread's `say` take an
`Idempotency-Key` header: up to 128 printable characters with no spaces. A second send with
the same key from the same asker starts nothing and gives back the first job (`say` also
says `"already": true`; `POST /api/jobs` answers `200` and not `201`). The key belongs to
the asker, so another person's identical key is another send, and it is kept in the job's
record for as long as the job is, so a retry after a restart still finds it. Sends that
carry a key go one at a time, so two arriving together are one.

A request that carries a person's token is never an admin's, even when it also
carries an admin's cookie.

## What is recorded

| Where | What it says |
| --- | --- |
| The job | Asked by `token:<name>`. Only an admin sees it, because a token is nobody's person |
| Spending | A *By who asked* panel, and `askers` in `GET /api/spend` |
| `token.created`, `token.revoked` | Who made or revoked it, and what it may do. Never the secret |
| `signin.refused` | A call with a token that is unknown, revoked, or lacks the scope, and where it came from |

A wrong token and a revoked one get the same answer, so a guess cannot learn
that a token once existed. The change record says which it was. A token that is
valid and lacks the scope gets 403.

## Revoke

Revoke a token from its row. It stops on its next request. It stays in the list,
with who ended it and when, because who held a token is part of the record.

A name can be used again once the token that held it is revoked. Records made
before and after are told apart by their time in the change record.

## The file

The file is written by the server alone, readable by its user only, and holds
each token's name, scopes, hash, who made it and when, and when it was last used.
The last-used time is saved at most once a minute.
