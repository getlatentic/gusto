# API tokens

A program that calls the server carries a token, not the shared password. A token may do only what it was given, is named in every record it makes, and stops working when an admin revokes it.

## Turn it on

Set `GUSTO_API_TOKENS` to the file the tokens are kept in. The server makes the file when the first token is made.

| Install | Setting |
| --- | --- |
| Compose | `GUSTO_API_TOKENS=/var/lib/gusto/tokens/tokens.json` in `.env` |
| Chart | `auth.apiTokens: true` |
| Playbook | `api_tokens: true` |

With tokens on, the API refuses the shared password. Without them it takes the shared password. The pages keep their own sign-in either way. The server alone writes the file, readable by its user only.

## Make one

An admin opens **Tokens** in the admin console and gives a name (lowercase letters, digits and dashes) and what the token may do.

A token may send tasks (`POST /api/jobs`), read spending (`GET /api/spend`), read the change record (`GET /api/changes`) or read metrics (`GET /metrics`). The secret is shown once. The server keeps only a hash, so make another token and revoke the old one if it is lost.

```bash
GUSTO_SERVER=https://gusto.example.com GUSTO_TOKEN=gusto_3fa1c2d0_... gusto submit ./task
```

`gusto submit` sends `GUSTO_TOKEN` when set, and `GUSTO_PASSWORD` otherwise. Any other caller sends `Authorization: Bearer <token>`.

## A token for a person

A person's computer gets a token that acts as that person, by the device flow. It needs the token file above and is not made at `/admin/tokens`. Where an identity provider is named, the shared password is nobody in particular and cannot approve a computer. Where none is, the shared password is the one owner, and a computer it approves acts as that owner.

1. The computer calls `POST /api/device` with `{"label":"Laptop"}` and receives a `device_code`, a `user_code` and an address.
2. The person signs in, opens `/device`, types the user code and approves it.
3. The computer polls `POST /api/device/token` with its `device_code` every few seconds. It receives `authorization_pending` until the person approves, then the token once. A code lasts ten minutes.

`gusto-helper login` (or `gusto login`) does these steps, then keeps the token in the keychain (macOS Keychain, Windows Credential Manager, Secret Service). With no keychain it uses a file in your settings folder, readable by you alone, and says so. `GUSTO_VAULT=file` asks for the file alone.

Run `gusto-helper login -server https://gusto.example.com`. `whoami` says who the token acts as (`GET /api/me`), and `logout` ends the token on the server; both take the same `-server`.

The token has one scope, `work`. It cannot send a task or read spending, the change record or metrics. A request with a person's token is never an admin's, even with an admin's cookie. The person sees and ends their own tokens at `/device`, and an admin sees all at `/admin/tokens`. A server with no token file has no device flow, and its pages answer 404.

With the token a program reads the person's own work as JSON. Another person's id answers as not found. [internal/web/protocol.go](../internal/web/protocol.go) lists every route.

| Route | Does |
| --- | --- |
| `GET /api/engines` | Lists the agents this server can start, whether each can run now, and the person's models |
| `GET`, `POST /api/instances` | Lists the person's agents, or starts one (the agent is the `harness` field): `{"harness": "...", "model": "...", "repo": "...", "setup": "...", "title": "..."}` (only `harness` required; answers `201`) |
| `GET /api/instances/{id}`, `.../threads`, `.../threads/{thread}` | Reads an agent, its threads, or one thread with its turns |
| `POST /api/instances/{id}/threads` | Opens a thread: `{"title": "..."}`. Answers `201` |
| `POST .../threads/{thread}/say` | Says something: `{"said": "..."}`. Answers `202` with the task |
| `GET /api/jobs/{id}`, `.../follow` | Reads a turn, or streams it as server-sent events (`step`, `reply`, `ended`) |
| `POST /api/jobs/{id}/stop` | Stops a turn, which ends `interrupted`. Answers `200`, or `409` when it is over ([STATES.md](STATES.md#stopping-a-task)) |
| `GET /api/instances/{id}/files` | Lists the folder's regular files, cut at 50,000 with `cut` set |
| `GET`, `PUT`, `DELETE .../file?path=` | Reads, writes or deletes a file. `PUT` and `DELETE` check `X-If-Hash` (a hash or `absent`) and answer `412` on a mismatch. `X-Mode: 755` keeps the executable bit. Over 64 MiB is `413`; a bad path is `400` |
| `POST .../hashes` | `{"paths": [...]}`, at most 5000. Returns each file's sha256 |

`GET /api/events` streams server-sent events about the person's own tasks. A connection first hears every unended task, and every task that ended after `?since=<unix seconds>` when given, then `ready`. After that each change is one event (`turn.queued`, `turn.started`, `turn.finished`, `turn.failed` or `turn.interrupted`) carrying `{"job","instance","thread","brief","state","at"}`.

`POST /api/jobs` and a thread's `say` take an `Idempotency-Key` header: up to 128 printable characters, no spaces. A second send with the same key from the same asker starts nothing and returns the first task (`say` adds `"already": true`; `POST /api/jobs` answers `200`, not `201`). The key is kept in the task's record.

## Records and revoking

- The task records `token:<name>` as its asker, visible to an admin only. Spending shows a *By who asked* panel and `askers` in `GET /api/spend`.
- `token.created` and `token.revoked` record who acted and what the token may do, never the secret. `signin.refused` records a call with an unknown, revoked or under-scoped token, and where it came from.
- A wrong token and a revoked one get the same answer. A valid token without the scope gets 403.

Revoke a token from its row. It stops on its next request and stays in the list with who ended it and when. A name can be used again after its token is revoked.
