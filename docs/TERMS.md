# Terms

Three terms, two of them easy to confuse.
| Term | Meaning |
| --- | --- |
| agent | The program that does the work: `claude`, `codex`, `opencode`, `pi`. A task names one in `job.json` |
| adapter | How to run an agent: the program, its arguments, its environment, and where the numbers sit in its output |
| configured agent | An agent plus what shapes its behaviour: skills, MCP servers, a prompt, a model policy, tool permissions |

A person shares, rents or sells a configured agent. Two people running `opencode` run the same agent and, usually, different configured agents.

## agent

A task names its agent in the `harness` field of `job.json`.

```json
{ "harness": "opencode" }
```

## adapter

An adapter is mechanical, not a preference. gusto writes one per agent, and every path in it comes from a real run of that agent. A guessed path reports no spend and nobody notices. An adapter travels inside a task, so a runner that has never heard of your agent can still run it.

```json
{ "name": "opencode", "program": "opencode",
  "argv": ["run", "--format", "json", "--model={model}", "{brief}"],
  "stream": "jsonl", "inputTokens": "part.tokens.input" }
```

## configured agent

### What one is on disk

A task carries the agent's directory (`harnesses/` in the task) and the task's setup.

```
job/
  harnesses/opencode/
    adapter.json            required; the only file gusto reads
    prompt.md               standing instructions
    model.json              the model policy
  setup/
    skills/<name>/SKILL.md  the skills it is given
    servers.json            the MCP servers it may call
```

- Only `adapter.json` is required. The other names are a convention. gusto does not interpret them.
- The adapter reaches the directory through `{harnessDir}`. For example, `"argv": ["--append-system-prompt-file", "{harnessDir}/prompt.md"]` hands claude its prompt.
- The setup is the skills and MCP servers a person chose. The adapter's `skills` and `mcp` say how the agent receives them.
- The same adapter with two setups is two configured agents.
- An agent directory may not have a `setup/` folder, because the digest lists the task's setup under that name.
- A bare adapter, `opencode.json`, is the smallest configured agent. It reads as a directory holding only `adapter.json`. A place that offers both `opencode.json` and `opencode/` is refused.

### Its digest

A configured agent is named by a SHA-256 digest over its files and its setup's. Anyone holding the directory computes the same string, with or without gusto. [FORMATS.md](FORMATS.md#the-configured-agent-digest) gives the steps.

Every run records the digest of the configured agent that ran. Two runs are the same agent's only when their digests match.

### Where it lives

- `gusto new` copies the configured agent from `GUSTO_HARNESSES` into the task's `harnesses/`. The runner takes the digest from the task's copy.
- An agent built into gusto is not carried. Its digest is of the adapter built into the runner that ran it.
- There is no registry. Share a configured agent as a git repository or a tarball. Both give the same digest.

### What it declares and what the run proves

The adapter, skills, prompt and model policy are files, and the digest covers them. The agent program, an MCP server's process and the model are references: a name in the adapter or in `servers.json`, resolved when the task runs. A digest over a reference does not prove what the reference resolved to. The directory declares, and the record of a run proves. Recording what each reference resolved to is not built yet.

### What it may not change

A configured agent is read, not written.

- The runner takes the digest before the agent starts.
- An agent that writes to its own directory shows up as changed when the task is verified.
- An agent given skills finds them where the adapter's `skills` says. That location is outside the task and the digest. The runner copies the skills there from the setup before each run.
- An agent that learns writes what it learned to the task's `out/`, as the record of that run.

## What the format will do

`job.json` names an agent, and the name finds a configured agent: in the task, in `GUSTO_HARNESSES`, or built in. The manifest names what it found, by digest. When something needs fetching by digest, that field will name the digest to fetch, as you run an image rather than a binary name. No registry exists until something needs one.
