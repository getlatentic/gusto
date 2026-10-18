# Terms

Three terms, two of them easy to confuse.
| Term | Meaning |
| --- | --- |
| harness | The program that does the work: `claude`, `codex`, `opencode`, `pi`. A job names one in `job.json` |
| adapter | How to run a harness: the program, its arguments, its environment, and where the numbers sit in its output |
| configured harness | A harness plus what shapes its behaviour: skills, MCP servers, a prompt, a model policy, tool permissions |

A person shares, rents or sells a configured harness. Two people running `opencode` run the same harness and, usually, different configured harnesses.

## harness

```json
{ "harness": "opencode" }
```

## adapter

An adapter is mechanical, not a preference. gusto writes one per harness, and every path in it comes from a real run of that harness. A guessed path reports no spend and nobody notices. An adapter travels inside a job, so a runner that has never heard of your harness can still run it.

```json
{ "name": "opencode", "program": "opencode",
  "argv": ["run", "--format", "json", "--model={model}", "{brief}"],
  "stream": "jsonl", "inputTokens": "part.tokens.input" }
```

## configured harness

### What one is on disk

A job carries the harness's directory and the job's setup.

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
- The setup is the skills and MCP servers a person chose. The adapter's `skills` and `mcp` say how the harness receives them.
- The same adapter with two setups is two configured harnesses.
- A harness directory may not have a `setup/` folder, because the digest lists the job's setup under that name.
- A bare adapter, `opencode.json`, is the smallest configured harness. It reads as a directory holding only `adapter.json`. A place that offers both `opencode.json` and `opencode/` is refused.

### Its digest

A configured harness is named by a SHA-256 digest over its files and its setup's. Anyone holding the directory computes the same string, with or without gusto. [FORMATS.md](FORMATS.md#the-configured-harness-digest) gives the steps.

Every run records the digest of the configured harness that ran. Two runs are the same harness's only when their digests match.

### Where it lives

- `gusto new` copies the configured harness from `GUSTO_HARNESSES` into the job's `harnesses/`. The runner takes the digest from the job's copy.
- A harness built into gusto is not carried. Its digest is of the adapter built into the runner that ran it.
- There is no registry. Share a configured harness as a git repository or a tarball. Both give the same digest.

### What it declares and what the run proves

The adapter, skills, prompt and model policy are files, and the digest covers them. The harness program, an MCP server's process and the model are references: a name in the adapter or in `servers.json`, resolved when the job runs. A digest over a reference does not prove what the reference resolved to. The directory declares, and the evidence of a run proves. Recording what each reference resolved to is not built yet.

### What it may not change

A configured harness is read, not written.

- The runner takes the digest before the harness starts.
- A harness that writes to its own directory shows up as changed when the job is verified.
- A harness given skills finds them where the adapter's `skills` says. That location is outside the job and the digest. The runner copies the skills there from the setup before each run.
- A harness that learns writes what it learned to the job's `out/`, as evidence of that run.

## What the format will do

`job.json` names a harness, and the name finds a configured harness: in the job, in `GUSTO_HARNESSES`, or built in. The manifest names what it found, by digest. When something needs fetching by digest, that field will name the digest to fetch, as you run an image rather than a binary name. No registry exists until something needs one.
