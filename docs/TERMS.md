# Terms

The words gusto's pages, commands and docs use, each for one thing. The last column is the older name the code still uses, where it differs.

## Who and where

| Word | Meaning | Older name in the code |
| --- | --- | --- |
| organization | A company, or one person. It holds billing, seats, the signing key and how people sign in. It has its own server | org |
| workspace | A team's space in an organization, such as Engineering or "Tosin Amuda's workspace". It holds the members, repositories, connectors, tasks and records. An organization has one workspace today, which holds everyone in it | org |
| person | Someone with an account in an organization: a member or an admin | user |
| computer | One person's machine in a workspace, kept between tasks: a VM, a container or the system sandbox, in the cloud or on their laptop | computer |
| node | One machine in gusto cloud. It runs several organizations' servers and their people's computers | cell, in what is stored or deployed: the `cell` database column, `cell-` in node names, `cells/`, the `X-Cell-*` headers and the AWS names |
| cluster | All the nodes of one gusto cloud | |
| gusto cloud | The hosted service for sign-up, sign-in and routing. It runs no tasks | front door, controller |
| operator | The person who runs a gusto cloud | owner |

## The work

| Word | Meaning | Older name in the code |
| --- | --- | --- |
| task | One piece of work for an agent: a brief, a repository, a check and a spend limit, in a directory of its own | job, in what is stored or sent: `job.json`, the `jobs/` folder, the `/jobs` and `/api/jobs` addresses, `job` in JSON and in container labels, the `gusto_jobs_*` metrics |
| working folder | The folder in a task's directory that the agent works in: the checked-out repository and what the agent changed | workspace, in what is stored or seen inside a sandbox: the `workspace/` folder, `/job/workspace`, `/sandbox/workspace`, a bundle's `workspace` branch and `workspaceDays` |
| agent's folder | The folder an agent works in across a conversation's turns, kept between them on the person's computer. Each turn of the conversation works in it | workspace |
| check | The command whose result says pass or fail | gate, in what is stored or sent: `gate` in `job.json`, the manifest and the audit record, `gate.log` and `gates.json` in `out/`, `gateCode` in a matrix table, and the task form's `gate` field |
| record | What a task leaves behind: hashed files, the check result and a signature. `gusto verify` re-checks it anywhere | evidence |
| sandbox | The isolated box one task runs in, on a computer | wall (the level) |
| room | A conversation that people and several agents share, with one folder | |
| turn | One message to an agent and the work it does for it. A turn is a task | |

## Agents

| Word | Meaning | Other names |
| --- | --- | --- |
| harness | The program that does the work: Claude Code, Codex, OpenCode, pi, Hermes or DeepSeek Agent. The code and the docs say harness. The pages say agent type, since a person picks one when they make an agent | agent type (pages), engine |
| adapter | How to run a harness: the program, its arguments, its environment, and where the numbers sit in its output | |
| agent | A harness with what shapes it: skills, connectors, a prompt and a model policy. A person makes one and names it, such as "Developer agent, made from Claude Code", and shares, rents or sells it | configured agent |

One harness makes many agents. Two people who run OpenCode run the same harness and, usually, different agents.

## harness

A task names its harness in the `harness` field of `job.json`.

```json
{ "harness": "opencode" }
```

## adapter

An adapter is mechanical, not a preference. gusto writes one per harness, and every path in it comes from a real run of that program. A guessed path reports no spend and nobody notices. An adapter travels inside a task, so a runner that has never heard of your harness can still run it.

```json
{ "name": "opencode", "program": "opencode",
  "argv": ["run", "--format", "json", "--model={model}", "{brief}"],
  "stream": "jsonl", "inputTokens": "part.tokens.input" }
```

## agent

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
- The same adapter with two setups is two agents.
- An agent directory may not have a `setup/` folder, because the digest lists the task's setup under that name.
- A bare adapter, `opencode.json`, is the smallest agent. It reads as a directory holding only `adapter.json`. A place that offers both `opencode.json` and `opencode/` is refused.

### Its digest

An agent is named by a SHA-256 digest over its files and its setup's. Anyone holding the directory computes the same string, with or without gusto. [FORMATS.md](FORMATS.md#the-agent-digest) gives the steps.

Every run records the digest of the agent that ran. Two runs are the same agent's only when their digests match.

### Where it lives

- `gusto new` copies the agent from `GUSTO_HARNESSES` into the task's `harnesses/`. The runner takes the digest from the task's copy.
- A harness built into gusto is not carried. Its digest is of the adapter built into the runner that ran it.
- There is no registry. Share an agent as a git repository or a tarball. Both give the same digest.

### What it declares and what the run proves

The adapter, skills, prompt and model policy are files, and the digest covers them. The harness, an MCP server's process and the model are references: a name in the adapter or in `servers.json`, resolved when the task runs. A digest over a reference does not prove what the reference resolved to. The directory declares, and the record of a run proves. Recording what each reference resolved to is not built yet.

### What it may not change

An agent is read, not written.

- The runner takes the digest before the agent starts.
- An agent that writes to its own directory shows up as changed when the task is verified.
- An agent given skills finds them where the adapter's `skills` says. That location is outside the task and the digest. The runner copies the skills there from the setup before each run.
- An agent that learns writes what it learned to the task's `out/`, as the record of that run.

## What the format will do

`job.json` names an agent, and the name finds its directory: in the task, in `GUSTO_HARNESSES`, or built in. The manifest names what it found, by digest. When something needs fetching by digest, that field will name the digest to fetch, as you run an image rather than a binary name. No registry exists until something needs one.
