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

The token has one scope, `work`. It can do nothing an organisation's token does:
it cannot send a job, read spending, the change record or metrics. `GET /api/me`
says who it acts as. The person sees their own tokens at `/device` and ends any of
them there, and an admin sees them all at `/admin/tokens`. Making one and ending one
are in the record of what changed on the platform. A server with no token file has
no device flow, and its pages answer 404.

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
