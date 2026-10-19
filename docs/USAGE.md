# Usage

The usage page lists every model call every agent made, added up by session. It is like the request log of an AI gateway. Open it at `/usage`.

The proxy is the only way out of a sandbox. So the page holds every call of every agent, whoever made it.

## What the page shows

Above the table are the totals for what matches the filter:
- what was spent, the calls, the tokens in and out, the share of the tokens in that the provider's cache answered, and the calls a cap refused.
- A call whose model has no price is not in the money. The page says "no price" for it and never shows it as zero. A total that leaves such calls out reads "at least".

The table has one row for each session. A session is the conversation of an agent a person set up (the instance and the thread), or a single task that was not part of one. A row shows:
- the last request (UTC), the session, who asked, the agent, the agent type, the cost, the providers, the models, the tokens, the cache hit rate and the calls.
- A link to the signed record of the session's latest task. A session with more tasks says how many more. Each of them is linked from the session page.

An agent type is the program: Claude Code, Codex, OpenCode, pi, Hermes or DeepSeek Agent. An agent is the named setup a person made, based on one of those types.

Click a session to see its calls, the latest first. Each call shows its time, task (linked to its record), model, provider, key name, tokens, cache hit rate, cost, status, the cap that refused it, and what the proxy translated.

While the index is still being filled, a notice says how many of the tasks it has read ("3 of 12 tasks"). An empty table at that time is not an empty month.

## Filters

| Filter | Query parameter | Notes |
| --- | --- | --- |
| Search | `q` | Part of a session, person, agent, agent type, provider or model. Case does not matter |
| Person | `person` | Exact |
| Team | `team` | Exact. The API only |
| Repository | `project` | Exact. The API only |
| Agent type | `agent_type` | Exact |
| Agent | `agent` | Exact |
| Provider | `provider` | Exact |
| Model | `model` | Exact |
| From | `from` | An RFC 3339 time, or a date (the start of that day, UTC) |
| To | `to` | An RFC 3339 time, which is not included, or a date, which is included |

Every filter narrows the table, the totals, the CSV and the calls of a session. On a session's page, the search box narrows its calls.

The sessions come by last request, the latest first. `sort` orders them from the greatest across all pages: `last` (the default), `cost`, `calls` or `tokens`. A session with no measured cost comes last in a sort by cost. Any other value is answered 400, naming the four. The column headings of the table set the sort and keep the filter, and the sorted one is marked.

The console has the same page at `/admin/usage`, next to Spending.

The page works without JavaScript. The selects send the form when they change, and the **Filter** button sends it otherwise.

## The API and CSV

All of these take the filter as query parameters. The lists take `limit` (1 to 1000, 50 by default), `after` (the `next` value of the last answer) and, for sessions, `sort`. The CSV has no pages, so it takes none of them. It streams every call that matches the filter, the latest first, and stops when the client leaves. Dates that cannot be read are answered 400.

| Call | Answer |
| --- | --- |
| `GET /api/usage/sessions` | `{"sessions":[...],"next":"..."}`. `next` is missing on the last page |
| `GET /api/usage/calls?session=<id>` | `{"calls":[...],"next":"..."}`, the latest first, only the calls that match the filter |
| `GET /api/usage/totals` | The calls, the unmeasured calls, the refused calls, the tokens and the dollars of the calls that match |
| `GET /api/usage/facets` | The people, teams, repositories, agent types, agents, providers and models the calls have |
| `GET /api/usage.csv` | Every call that matches, one line each, with a header line |

```bash
curl -H "Authorization: Bearer $TOKEN" "$GUSTO/api/usage/sessions?agent_type=Codex&from=2026-10-01&sort=cost&limit=20"
curl -H "Authorization: Bearer $TOKEN" "$GUSTO/api/usage/calls?session=20261013-0924-pi:1"
curl -H "Authorization: Bearer $TOKEN" "$GUSTO/api/usage/totals?person=ada@acme.example&to=2026-10-31"
curl -H "Authorization: Bearer $TOKEN" "$GUSTO/api/usage/facets"
curl -H "Authorization: Bearer $TOKEN" -o usage.csv "$GUSTO/api/usage.csv?q=deepseek&from=2026-10-01"
```

An answer given while the index is being filled has a `Usage-Index-Building: 3 of 12 tasks` header.

The CSV columns are `time, session, task, line, person, team, project, agent, agent_type, provider, key, model, input_tokens, output_tokens, cached_tokens, cost_usd, status, took_ms, refused, translated`.
- `time` is RFC 3339 in UTC.
- An empty `cost_usd` is a call with no price. An empty count is a count the answer did not carry. Neither is zero.
- A text cell that begins with `=`, `+`, `-`, `@`, a tab or a carriage return has a single quote put before it, so that a spreadsheet does not run it.

## Who sees what

- An administrator sees everyone's rows.
- Anyone else sees only their own rows. The server sets the person filter to the signed-in person on every page and call. A `person` parameter cannot widen it. Another person's session is answered as one that does not exist, by its id, by search, by the CSV and by the facets.
- A token that belongs to a person (from the device flow) sees that person's rows, whoever the person is. A person's token is never an administrator's.
- A token with the `spend` scope sees all rows, as it sees the whole spend report. With no API tokens on the server, the shared password does too.
- The API also takes a signed-in session, so that the **Download CSV** link on the page works.

## Where the rows come from

The proxy writes each call to the task's `out/proxy.jsonl`, and that file stays the record the manifest hashes. The usage rows are an index built from those files. A row is the call's place in its file (the task and the line), so reading a file twice adds nothing twice.
- `gusto usage reindex` rebuilds the index from the files.
- Removing a task's files, as retention does, keeps the task's rows in the index.

## The database

| `GUSTO_DATABASE_URL` | The index is |
| --- | --- |
| empty | A SQLite file, `usage.db`, in the data directory |
| `postgres://...` | In Postgres |

A `search_path` parameter in the Postgres URL gives each organisation its own schema on a shared Postgres.
