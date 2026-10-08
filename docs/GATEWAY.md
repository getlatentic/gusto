# The gateway

Every call an agent makes to a model leaves its sandbox through one place: the proxy. A sandbox can reach nothing
else, because its network policy allows the proxy and no other host. So the proxy is where a deployment holds
the keys, counts the spend, stops a call over its cap, and keeps the record. This note says what the proxy does
to a call, and where the next parts of it go (planning#523).

## What happens to a call

```
harness (unchanged) ──▶ proxy ──▶ provider
   in its sandbox        │
                         ├─ trades the job's token for the credential's key
                         ├─ refuses the call when a cap is spent
                         ├─ translates the call when the provider answers another shape
                         └─ counts the tokens and the cost, and writes the record
```

1. The harness calls the address it was given, with the job's token where its key would go.
2. The proxy finds the job and its credential by the token. A container cannot choose a credential.
3. The ledger checks the job's, the team's and the organisation's caps.
4. The proxy sends the call. It goes as it came, or translated (see below).
5. The proxy counts the answer from the provider's own usage, and writes one line to `out/proxy.jsonl`.

## Translation

Harnesses call a model in three shapes:

| Shape | Who calls in it | Path |
| --- | --- | --- |
| Chat Completions | OpenCode, Pi, Hermes, DSH | `.../chat/completions` |
| Messages | Claude Code | `.../messages` |
| Responses | Codex | `.../responses` |

Many providers answer Chat Completions alone, such as the Gemma models on Amazon Bedrock. A call in Messages or
Responses to such a provider is translated into Chat Completions, and the answer, streamed or whole, is translated
back. So Claude Code and Codex can use any model a deployment connects. The code is `internal/wire`.

A credential turns translation on by naming the shapes its provider answers:

```json
{"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": "...",
 "wires": ["chat"], "maxOutput": 8192, "harnesses": ["opencode", "pi", "hermes", "claude", "codex"]}
```

| Field | What it says |
| --- | --- |
| `wires` | The shapes the provider answers: `chat`, `messages`, `responses`. A call in another shape is translated when `chat` is one of them. With no `wires`, a call goes as it came, as it always has. |
| `maxOutput` | The most the provider lets a model write in one answer. A translated call that asks for more is held to it, because a harness asks for what its own provider allows. |
| `harnesses` | The harnesses the credential serves. With none, it serves every harness that its shapes reach. |

The pages offer a harness a model when the model's credential reaches the harness's shape, as it is or translated.

What one shape has and Chat Completions cannot carry is left out, not imitated:

- Anthropic's thinking blocks, cache hints, context management and effort.
- Codex's reasoning, its encrypted history, a tool namespace, and the tools OpenAI runs itself, such as web search.
- A picture in a tool's result. The result says that an image was left out.

A tool Codex calls with free text, such as a patch, becomes a function with one text argument, and goes back to
Codex as free text. An instruction given in the middle of a conversation becomes a user's message, because many open
models' templates allow a system message only first. A tool call that the harness did not answer is answered with
"(no result)", because a provider refuses a call left open.

The record of a translated call has `"translated": "messages"` or `"responses"`. Its tokens and cost come from the
provider's own answer, like any other call.

### How it is tested

- `internal/wire` reads requests recorded from Claude Code 2.1.270 and Codex 0.154.0, with their text replaced. It
  writes Chat Completions answers back in both shapes, streamed and whole, with interleaved tool calls, a refusal and
  a stream that is cut off.
- `internal/proxy` sends both shapes through the proxy to a provider that answers Chat Completions alone.
- `go test -tags realharnesses -run RealHarnesses ./internal/proxy` runs the real `claude` and `codex` programs
  through the proxy to such a provider. Each runs a tool, sends its result back, and finishes the turn. It needs
  both programs installed.

## Tools

A job's harness is given one MCP server, the gateway, and the gateway calls the real servers
(`internal/gateway`). The servers' addresses and credentials stay in the gateway and are never in the
sandbox.

| What | How |
| --- | --- |
| Finding a tool | The harness is shown three tools: `tool_search`, `tool_schema` and `tool_call`. A job reads the definitions of the tools it uses, and not of the others. In `all` mode it is shown every tool it may use, as a harness is given a server directly. |
| Policy | A rule names a server, a tool, a person or a team, as patterns, and allows or refuses. The first rule that matches decides, and a call no rule matches is allowed. A tool the policy takes from a caller is not found, and asking for it by name is refused with the rule's reason. |
| The record | `out/gateway.jsonl` has a line for each listing, search, schema and call: who, which server and tool, the outcome (`ok`, `error`, `refused`, `failed`), how long, and how many bytes went in and came back. It does not hold what was sent or what came back. A listing says how many tools there were and about how many tokens showing all of them would cost, against what the job was shown. |
| Reaching a server | Streamable HTTP, answered as JSON or as an event stream, with the session started again when the server drops it. A redirect is not followed. An address on a private network is refused where the connection is made, so a name that answers with a private address later is refused too. |
| A call that cannot be written down | Is not made. |

The tokens are estimated at four characters each, which is a rule of thumb and not a count.

**How a job reaches it.** A setup's integrations put the servers they bring behind the gateway: their
definitions are written to the job's `setup/gateway.json`, for its owner alone, and `job.json` names
them under `setup.gateway`. The runner gives the harness one server, `gateway`, at the proxy's address
with the job's token, and the proxy answers `/mcp` for the job whose token it is. A job that needs no
model but has servers behind the gateway is given a token that stands in for no credential.
A job on a deployment with no proxy is refused by name.

**Settings.** `HAAS_TOOL_POLICY` is the policy file, `HAAS_GATEWAY_MODE` is `discover` (the default) or
`all`, and `HAAS_GATEWAY_PRIVATE=1` lets the servers be on a private network. All three, with the
policy's fingerprint, are in the settings the change record watches.

**Checked against real clients.** `go test -tags realclients -run RealClient ./internal/gateway` connects
`claude` and the protocol's own TypeScript client library to the gateway. Running the SDK found a
schema that the in-process tests had passed and the library refused, so both are kept.

## What comes next

| Part | Issue |
| --- | --- |
| Redaction of personal data and secrets in what is sent and received | planning#518 |
| New models, tools and skill sets off until an admin enables them | planning#517 |
| Tools reached through the gateway, with a policy for each caller | planning#519 |
| Settings that change without a restart | planning#520 |
| The tokens a model sampled kept, for evaluation and training | planning#521 |

Each goes in the proxy, beside translation, because every call already passes through it.
