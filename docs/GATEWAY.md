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

A credential with a bearer token is taken to answer Chat Completions and Responses, and one with Anthropic's header
to answer Messages. A call in a shape it does not answer is translated into Chat Completions when it answers that, so
Claude Code can use any OpenAI-compatible key with no more said. A credential names its shapes when they are fewer:

```json
{"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": "...",
 "wires": ["chat"], "maxOutput": 8192}
```

| Field | What it says |
| --- | --- |
| `wires` | The shapes the provider answers: `chat`, `messages`, `responses`. A call in another shape is translated when `chat` is one of them. With no `wires`, the shapes are the ones its `auth` implies. |
| `maxOutput` | The most the provider lets a model write in one answer. A translated call that asks for more is held to it, because a harness asks for what its own provider allows. |

The pages offer every harness every model. Which pairs work well is for the person to find out.

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
| A server that is a program | Run in a box of its own, started when the job first uses it and ended with the job, spoken to over its standard input and output. See below. |

The tokens in the record are estimated at four characters each, which is a rule of thumb and not a count. The section below
has what real harnesses read.

**Measured on real harnesses**, on 2026-10-08, with an MCP server of 60 tools of about 200 tokens each (a description and a
schema of the size real servers have), given directly through the gateway's `all` mode and through its three tools. Input tokens
the model read for one short turn, in three calls each:

| Harness (model) | No tools | All 60 tools | The three tools |
| --- | --- | --- | --- |
| Claude Code 2.1.270 (haiku) | 40401, 40401, 40403 | 41109, 41111, 41113 | 40501, 40502, 40734 |
| Codex (gpt-5.5) | 22849 ×3 | 22864 ×3 | 22883 ×3 |
| OpenCode (big-pickle) | 20901, 20907, 25359 | 30176, 30181, 39792 | 25660, 25661, 25666 |

Claude Code and Codex do not put the definitions of an MCP server's tools in the first request: Claude Code adds about 710
tokens for the 60 and Codex about 15, since each loads a tool's definition when it is needed. For them the gateway saves a few
hundred tokens at most, and its worth is the control, the credentials kept outside the sandbox and the record. OpenCode puts every
definition in the request, and the 60 cost it about 9,000 tokens more than none, against about 5,000 for the gateway's three on
the same runs; the free model's counts move by several thousand between calls, so the OpenCode figures say a saving of the order
of 5,000 to 9,000 tokens for 60 tools and not a precise one. The other harnesses were not measured.
`HAAS_MEASURE=1 go test -tags realclients -run Measure -v ./internal/gateway` repeats it: it makes short calls to the models those
programs are signed in to.

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

## Programs

A plugin's MCP server may be a program (`"type": "stdio"`). It is somebody else's code, so it runs only when an administrator allows
the programs of that version, and then in a box that is not the job's:

| What the box has | |
| --- | --- |
| The plugin | its folder, mounted read-only at `/plugin` (`${PLUGIN_ROOT}`) |
| A place to write | a folder of this job's, at `/data` (`${PLUGIN_DATA}`), which goes with the job and is not kept between jobs |
| The rest of the file system | read-only, with 64 MiB of scratch space in `/tmp` |
| The network | none. `HAAS_TOOLS_NETWORK=open` gives it the runtime's own network, which lets a program send what it is given to any host on it, and is for an administrator to choose |
| Privilege | no capability, no new privilege, user 65534, at most 0.5 CPU, 512 MiB and 128 processes (`HAAS_TOOL_CPUS`, `HAAS_TOOL_MEMORY`) |
| The job | nothing: not its folder, its files, its token, the proxy or the docker socket |

The image is `HAAS_TOOLS_IMAGE`, or the job image. The image's own entry is replaced by the plugin's command, and the plugin's
placeholders are expanded the way the format says. A plugin that needs a program the image lacks, a network it is not given, or a
secret (the format has no way to give one) cannot run yet. A job that runs in the server's own process (`HAAS_SANDBOX=here`) has no
container to put a program in, and runs none.

The gateway speaks to the program a line of JSON at a time, answers a ping, tells it this client does not do what else it asks, ends
it with the job (by closing its input, and then by removing the container by name), and says what it wrote to its error output when
it did not start or died in a call.

## Masking personal data and secrets

`HAAS_REDACTION` is a file of the kinds to mask for the whole deployment: `{"patterns": ["email", "card", "key"]}`. It is read
again while the server runs ([LIVE.md](LIVE.md)); a name that is not a kind is refused, and a server with no proxy refuses to start
with it, since its jobs call their model directly and nothing could mask what they send.

| Kind | What it finds |
| --- | --- |
| `email` | an email address |
| `card` | 13 to 19 digits, which may have single spaces or hyphens between them, that pass the card check digit (Luhn); a number that fails it is an identifier and is left |
| `key` | private key blocks, AWS access key ids, GitHub tokens, Anthropic and OpenAI style keys (`sk-`), Slack, Google, Stripe, JSON web tokens, and `Bearer` followed by a long token |

A match is replaced by `[redacted:<kind>]`.

| Where | What is masked |
| --- | --- |
| A request to a model | Every string of the request, before it is translated, so the provider sees the mask whatever shape the harness called in. A card written as a bare JSON number becomes the string. |
| An answer from a model | Every string of a whole answer, and each event of a stream as it is sent, so a stream still arrives event by event. |
| What a tool answers | Before the harness sees it, so it is not in the conversation, the transcript or the next request. What is sent to a tool is not masked, because a tool may need the address it is asked about. |

Each call's line in `out/proxy.jsonl` has `redacted` (the request) and `redactedAnswer`, the count of each kind, and a tool call's
line in `out/gateway.jsonl` has `redacted`. What was masked is not kept.

What it does not do: a value a model writes across two events of a stream is whole in neither, so it is not found. A value
written with JSON's `\u` escapes for its letters is read unescaped, but a harness that spells a secret in pieces, or encodes it,
is not caught by patterns. These are patterns and not a guarantee, and a kind of data they do not describe is not masked.

**Cost**, measured with `go test -bench . ./internal/redact` on one machine: about 5 ms for a megabyte of ordinary conversation
(190 MB/s), and 120 ms for a megabyte that is almost all secrets. A message is looked at for a pattern only where it shows a sign
of it, which is why ordinary text costs so little.

## What comes next

| Part | Issue |
| --- | --- |
| New models, tools and skill sets off until an admin enables them | planning#517 |
| Tools reached through the gateway, with a policy for each caller | planning#519 |
| Settings that change without a restart | planning#520 |
| The tokens a model sampled kept, for evaluation and training | planning#521 |

Each goes in the proxy, beside translation, because every call already passes through it.
