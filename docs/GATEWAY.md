# The gateway

Every call an agent makes to a model leaves its sandbox through the proxy. The sandbox network policy allows the proxy and no other host. The proxy holds the keys, counts the spend, stops a call over its cap and keeps the record.

The harness calls the address it was given, with the job's token where its key would go. The proxy finds the job and its credential by the token, so a container cannot choose a credential. The ledger checks the job's, the team's and the organisation's caps. The proxy sends the call as it came, or translated, counts the answer from the provider's own usage and writes one line to `out/proxy.jsonl`.

## Translation

| Shape | Who calls in it | Path |
| --- | --- | --- |
| Chat Completions | OpenCode, Pi, Hermes, DSH | `.../chat/completions` |
| Messages | Claude Code | `.../messages` |
| Responses | Codex | `.../responses` |

Many providers answer Chat Completions alone, such as the Gemma models on Amazon Bedrock. The proxy translates a Messages or Responses call to Chat Completions, and translates the answer back, streamed or whole. Claude Code and Codex can then use any model a deployment connects. The code is `internal/wire`.

A bearer-token credential answers Chat Completions and Responses. An Anthropic-header credential answers Messages. A credential with fewer shapes says so:

```json
{"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": "...",
 "wires": ["chat"], "maxOutput": 8192}
```

- `wires` lists the shapes the provider answers: `chat`, `messages`, `responses`. A call in another shape is translated when `chat` is one of them.
- `maxOutput` is the most the provider lets a model write in one answer. A translated call that asks for more is held to it.

Translation leaves out what Chat Completions cannot carry: Anthropic's thinking blocks, cache hints, context management and effort; Codex's reasoning, encrypted history, tool namespace and tools OpenAI runs itself, such as web search; and a picture in a tool's result, which says an image was left out.

A Codex free-text tool, such as a patch, becomes a function with one text argument. A mid-conversation instruction becomes a user message. A tool call the harness did not answer is answered with "(no result)". A translated call's record has `"translated": "messages"` or `"responses"`.

Tests use requests recorded from Claude Code 2.1.270 and Codex 0.154.0. `go test -tags realharnesses -run RealHarnesses ./internal/proxy` runs the real `claude` and `codex` through the proxy.

## Tools

A job's harness is given one MCP server, the gateway. The gateway calls the real servers (`internal/gateway`). Their addresses and credentials never enter the sandbox.

| What | How |
| --- | --- |
| Finding a tool | The harness sees `tool_search`, `tool_schema` and `tool_call`. In `all` mode it sees every tool it may use |
| Policy | A rule names a server, tool, person or team as patterns, and allows or refuses. The first match decides. A call no rule matches is allowed. A refused tool is not found, and asking for it by name is refused with the rule's reason |
| The record | `out/gateway.jsonl` has a line per listing, search, schema and call: who, server, tool, outcome (`ok`, `error`, `refused`, `failed`), duration and bytes. It holds no content. Token figures are estimated at four characters each |
| Reaching a server | Streamable HTTP, JSON or event stream. Redirects are not followed. A private-network address is refused where the connection is made |

A setup's integrations put their servers behind the gateway. Their definitions go to the job's `setup/gateway.json`, and `job.json` names them under `setup.gateway`. A job on a deployment with no proxy is refused by name.

Settings: `GUSTO_TOOL_POLICY` is the policy file. `GUSTO_GATEWAY_MODE` is `discover` (default) or `all`. `GUSTO_GATEWAY_PRIVATE=1` lets servers be on a private network.

Measured on 2026-10-08 with 60 tools of about 200 tokens each: Claude Code and Codex load a tool's definition when needed, so 60 tools added about 710 and about 15 input tokens. OpenCode puts every definition in the request. The 60 tools cost it about 9,000 tokens, and the three gateway tools about 5,000, with counts that vary by thousands between calls. `GUSTO_MEASURE=1 go test -tags realclients -run Measure -v ./internal/gateway` repeats it.

### Programs

A plugin's MCP server may be a program (`"type": "stdio"`). It starts on first use, ends with the job, and runs only when an administrator allows the programs of that version, in a box that is not the job's:

| The box | Has |
| --- | --- |
| The plugin | its folder, read-only at `/plugin` (`${PLUGIN_ROOT}`) |
| A place to write | `/data` (`${PLUGIN_DATA}`), kept for this job only; the rest is read-only, with 64 MiB in `/tmp` |
| The network | none. `GUSTO_TOOLS_NETWORK=open` gives it the runtime's network, so a program can send what it is given to any host on it |
| Privilege | no capability, no new privilege, user 65534, at most 0.5 CPU, 512 MiB and 128 processes (`GUSTO_TOOL_CPUS`, `GUSTO_TOOL_MEMORY`) |
| The job | nothing: not its folder, files, token, the proxy or the docker socket |

The image is `GUSTO_TOOLS_IMAGE`, or the job image. A plugin that needs a program the image lacks, a network it is not given, or a secret cannot run yet. A job with `GUSTO_SANDBOX=here` runs no program.

## Masking personal data and secrets

`GUSTO_REDACTION` is a file of the kinds to mask for the deployment: `{"patterns": ["email", "card", "key"]}`. It reloads while the server runs ([LIVE.md](LIVE.md)). An unknown kind is refused. A server with no proxy refuses to start with it.

| Kind | What it finds |
| --- | --- |
| `email` | an email address |
| `card` | 13 to 19 digits, with single spaces or hyphens allowed, that pass the Luhn check |
| `key` | private key blocks, AWS access key ids, GitHub tokens, Anthropic and OpenAI style keys (`sk-`), Slack, Google, Stripe, JSON web tokens, and `Bearer` followed by a long token |

A match becomes `[redacted:<kind>]`. The proxy masks every string of a model request before translation, and every string of a model answer, event by event for a stream. It masks what a tool answers before the harness sees it. What is sent to a tool is not masked.

`out/proxy.jsonl` has `redacted` and `redactedAnswer` counts for each kind, and `out/gateway.jsonl` has `redacted`. What was masked is not kept.

These are patterns, not a guarantee. A value split across two stream events, spelled in pieces or encoded is not caught. Masking costs about 5 ms for a megabyte of ordinary text and 120 ms for a megabyte of almost all secrets.

Planned for the proxy: models, tools and skill sets off until an admin enables them (planning#517), a policy for each caller on tools (planning#519), settings that change without a restart (planning#520), and sampled tokens kept for evaluation and training (planning#521).
