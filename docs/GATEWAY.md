# The gateway

Every call an agent makes to a model leaves its sandbox through the proxy. The sandbox network policy allows the proxy and no other host. The proxy holds the keys, counts the spend, stops a call over its cap and keeps the record.

The agent calls the address it was given, with the task's token where its key would go. The proxy finds the task and its credential by the token, so a container cannot choose a credential. The ledger checks the task's, the team's and the organization's caps. The proxy sends the call as it came, or translated, counts the answer from the provider's own usage and writes one line to `out/proxy.jsonl`.

## Translation

| Shape | Who calls in it | Path |
| --- | --- | --- |
| Chat Completions | OpenCode, Pi, Hermes, DSH | `.../chat/completions` |
| Messages | Claude Code | `.../messages` |
| Responses | Codex | `.../responses` |

Many providers answer Chat Completions alone, such as the Gemma models on Amazon Bedrock. The proxy translates a Messages or Responses call to Chat Completions, and translates the answer back, streamed or whole. Claude Code and Codex can then use any model a deployment connects. The code is `pkg/wire`.

A bearer-token credential answers Chat Completions and Responses. An Anthropic-header credential answers Messages. A credential with fewer shapes says so:

```json
{"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": "...",
 "wires": ["chat"], "maxOutput": 8192}
```

- `wires` lists the shapes the provider answers: `chat`, `messages`, `responses`. A call in another shape is translated when `chat` is one of them.
- `maxOutput` is the most the provider lets a model write in one answer. A translated call that asks for more is held to it.

A provider that answers different shapes at different addresses, under one key, is one credential with `endpoints`. DeepSeek answers Chat Completions and Responses at its root and Messages under `/anthropic`:

```json
{"address": "https://api.deepseek.com", "auth": "bearer", "key": "...",
 "endpoints": [{"address": "https://api.deepseek.com/anthropic", "auth": "anthropic"}]}
```

- A call goes to the first address that answers its shape. When none does, it is translated at the first address that answers `chat`.
- Each endpoint takes `address`, `auth` and `wires`, as the credential does. The key is the credential's.
- The model is then one name, such as `deepseek/deepseek-flash`, for every engine. On the Models page an admin adds an address as OpenAI-compatible, Anthropic-compatible or custom.

Translation leaves out what Chat Completions cannot carry: Anthropic's thinking blocks, cache hints, context management and effort; Codex's reasoning, encrypted history, tool namespace and tools OpenAI runs itself, such as web search; and a picture in a tool's result, which says an image was left out.

A Codex free-text tool, such as a patch, becomes a function with one text argument. A mid-conversation instruction becomes a user message. A tool call the agent did not answer is answered with "(no result)". A translated call's record has `"translated": "messages"` or `"responses"`.

Tests use requests recorded from Claude Code 2.1.270 and Codex 0.154.0. `go test -tags realharnesses -run RealHarnesses ./pkg/proxy` runs the real `claude` and `codex` through the proxy.

## Tools

A task's agent is given one MCP server, the gateway. The gateway calls the real servers (`pkg/gateway`). Their addresses and credentials never enter the sandbox.

| What | How |
| --- | --- |
| Finding a tool | The agent sees `tool_search`, `tool_schema` and `tool_call`. In `all` mode it sees every tool it may use |
| Policy | A rule names a server, tool, person or team as patterns, and allows or refuses. The first match decides. A call no rule matches is allowed. A refused tool is not found, and asking for it by name is refused with the rule's reason |
| The record | `out/gateway.jsonl` has a line per listing, search, schema and call: who, server, tool, outcome (`ok`, `error`, `refused`, `failed`), duration and bytes. It holds no content. Token figures are estimated at four characters each |
| Reaching a server | Streamable HTTP, JSON or event stream. Redirects are not followed. A private-network address is refused where the connection is made |

A setup's integrations put their servers behind the gateway. Their definitions go to the task's `setup/gateway.json`, and `job.json` names them under `setup.gateway`. A task on a deployment with no proxy is refused by name.

Settings: `GUSTO_TOOL_POLICY` is the policy file. `GUSTO_GATEWAY_MODE` is `discover` (default) or `all`. `GUSTO_GATEWAY_PRIVATE=1` lets servers be on a private network.

### Connectors

A connector is an app's MCP server that asks for OAuth. Each person signs in to it with their own account, and their agents call it as them (`internal/connector`).

1. An admin turns the app on at `/admin/connectors`. Every app is off until then.
2. The person opens Connections, `/connections`, and presses Connect. The app's own sign-in page opens.
3. The app sends them back to `/connections/callback`. gusto trades the code for tokens and keeps them sealed in `GUSTO_CONNECTORS`.
4. The person's next task has the app behind its gateway. `job.json` names it under `setup.connectors` and `setup.gateway`.
5. At each call the gateway asks for the person's token, renewed when it is about to run out, and sends it to the app. The agent and the sandbox never hold it.

| What | How |
| --- | --- |
| Finding how to sign in | The MCP server's 401 names its resource metadata (RFC 9728), which names the authorization server (RFC 8414 or OpenID Connect). Only https is read, or http on this computer |
| How gusto is known to the app | The operator's client when one is in `GUSTO_CONNECTOR_CLIENTS`. Otherwise the client document at `/connectors/client.json`, when the app reads one and the server is on https. Otherwise a client the app registers (RFC 7591), kept for next time. An operator's secret is sent as basic authentication when the app names it, and in the form otherwise |
| The sign-in | PKCE with S256, a `state` bound to the person, and the `resource` parameter. A sign-in not finished in 10 minutes is forgotten |
| Whose token | The person who asked for the task, read from the task's state. A room's agent works for everyone in the room, and is given nobody's sign-ins. A task another person pays for is given none |
| Which agents | An agent that takes the gateway in its arguments or on its session. Pi reads its servers from a file, which a sandbox does not carry, so it is given none |
| The vault | AES-GCM, with a key made the first time and kept beside the tokens, readable by the server's user alone |

The catalog, asked on 2026-10-31: Linear, Notion, Figma, Canva and Stripe need no setup. Trello, asked on 2026-11-01, needs none either, and is asked for its boards, its members and offline access, not its inbox or planner. GitHub, Asana and HubSpot need a client the operator registers with them, with `<GUSTO_PUBLIC_URL>/connections/callback` as its redirect. GitHub is asked for `repo`, `read:org` and `read:user`, not the ten scopes its server lists. `GUSTO_MEASURE=1 go test -run Catalog ./internal/connector` asks each one again.

To add GitHub to gusto cloud:

1. Make an OAuth app under the organization's GitHub settings, Developer settings, OAuth Apps.
2. Set the callback URL to `https://<GUSTO_CLOUD_DOMAIN>/connections/callback` and turn on wildcard matching for it, so that every organization's `<slug>.<domain>` matches.
3. Make a client secret.
4. Run `deploy/aws/openshell/put-connector-client.sh github <client id>` and type the secret at the prompt.
5. Deploy. Each organization's server reads the clients when it starts, and its admin can then turn GitHub on.

A GitHub organization that restricts OAuth apps asks its owner to approve the app the first time a member connects.

Settings: `GUSTO_CONNECTORS` is the folder of sealed sign-ins, and turns connectors on. `GUSTO_PUBLIC_URL` is the address an app sends a person back to. `GUSTO_CONNECTOR_CLIENTS` is a JSON file of operator clients, `{"github": {"id": "…", "secret": "…"}}`, or `ssm:<name>` for a SecureString parameter holding the same. A parameter that cannot be read leaves those apps off and is logged; the server still starts. `GUSTO_CONNECTOR_APPS` is a JSON file of more apps, `[{"id": "wiki", "name": "Wiki", "what": "…", "url": "https://…"}]`. gusto cloud sets the first two for each organization.

Measured on 2026-10-08 with 60 tools of about 200 tokens each: Claude Code and Codex load a tool's definition when needed, so 60 tools added about 710 and about 15 input tokens. OpenCode puts every definition in the request. The 60 tools cost it about 9,000 tokens, and the three gateway tools about 5,000, with counts that vary by thousands between calls. `GUSTO_MEASURE=1 go test -tags realclients -run Measure -v ./pkg/gateway` repeats it.

### Programs

A plugin's MCP server may be a program (`"type": "stdio"`). It starts on first use, ends with the task, and runs only when an administrator allows the programs of that version, in a box that is not the task's:

| The box | Has |
| --- | --- |
| The plugin | its folder, read-only at `/plugin` (`${PLUGIN_ROOT}`) |
| A place to write | `/data` (`${PLUGIN_DATA}`), kept for this task only; the rest is read-only, with 64 MiB in `/tmp` |
| The network | none. `GUSTO_TOOLS_NETWORK=open` gives it the runtime's network, so a program can send what it is given to any host on it |
| Privilege | no capability, no new privilege, user 65534, at most 0.5 CPU, 512 MiB and 128 processes (`GUSTO_TOOL_CPUS`, `GUSTO_TOOL_MEMORY`) |
| The task | nothing: not its folder, files, token, the proxy or the docker socket |

The image is `GUSTO_TOOLS_IMAGE`, or the task image. A plugin that needs a program the image lacks, a network it is not given, or a secret cannot run yet. A task with `GUSTO_SANDBOX=here` runs no program.

## Masking personal data and secrets

`GUSTO_REDACTION` is a file of the kinds to mask for the deployment: `{"patterns": ["email", "card", "key"]}`. It reloads while the server runs ([LIVE.md](LIVE.md)). An unknown kind is refused. A server with no proxy refuses to start with it.

| Kind | What it finds |
| --- | --- |
| `email` | an email address |
| `card` | 13 to 19 digits, with single spaces or hyphens allowed, that pass the Luhn check |
| `key` | private key blocks, AWS access key ids, GitHub tokens, Anthropic and OpenAI style keys (`sk-`), Slack, Google, Stripe, JSON web tokens, and `Bearer` followed by a long token |

A match becomes `[redacted:<kind>]`. The proxy masks every string of a model request before translation, and every string of a model answer, event by event for a stream. It masks what a tool answers before the agent sees it. What is sent to a tool is not masked.

`out/proxy.jsonl` has `redacted` and `redactedAnswer` counts for each kind, and `out/gateway.jsonl` has `redacted`. What was masked is not kept.

These are patterns, not a guarantee. A value split across two stream events, spelled in pieces or encoded is not caught. Masking costs about 5 ms for a megabyte of ordinary text and 120 ms for a megabyte of almost all secrets.

Planned for the proxy: models, tools and skill sets off until an admin enables them (planning#517), a policy for each caller on tools (planning#519), settings that change without a restart (planning#520), and sampled tokens kept for evaluation and training (planning#521).
