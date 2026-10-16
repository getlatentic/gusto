# Terms

Three things, and two of them keep borrowing one word.

## harness

The program that does the work: `claude`, `codex`, `opencode`, `pi`. Nothing
more. A job names one in `job.json`:

```json
{ "harness": "opencode" }
```

## adapter

How to run a harness: the program, its arguments, the environment it needs, and
where the numbers sit in whatever it prints.

An adapter is mechanical, not a preference. It is ours to write, one per
harness, and every path in one comes off a real run of that harness — a guessed
path reports no spend and nobody notices. An adapter travels inside a job, so a
runner that has never heard of your harness can still run it.

```json
{ "name": "opencode", "program": "opencode",
  "argv": ["run", "--format", "json", "--model={model}", "{brief}"],
  "stream": "jsonl", "inputTokens": "part.tokens.input" }
```

## configured harness

A harness plus what shapes its behaviour: skills, MCP servers, a prompt, a model
policy, tool permissions.

This is the thing a person shares, rents or sells. Two people running `opencode`
run the same harness and, in every way that matters to them, different
configured harnesses.

### What one is on disk

A configured harness is two things a job carries: the harness's directory, and
the job's setup.

```
job/
  harnesses/opencode/
    adapter.json          how to run it: the adapter, and the only file gusto reads
    prompt.md             standing instructions
    model.json            the model policy
  setup/
    skills/<name>/SKILL.md  the skills it is given
    servers.json            the MCP servers it may call
```

The directory holds `adapter.json`, which is required, and whatever the adapter
points its harness at. The other names are a convention rather than a schema:
gusto does not interpret them. The adapter reaches them through `{harnessDir}`,
which is filled with the directory's path, so `"argv":
["--append-system-prompt-file", "{harnessDir}/prompt.md"]` hands claude its
prompt. Every file in the directory is part of the configured harness, named
above or not.

The setup is the skills and MCP servers a person chose, carried in the job's
`setup/` and handed to the harness the way its adapter says (`skills`, `mcp`).
It is part of the configured harness too: the same adapter given two setups is
two configured harnesses. A harness directory may not have a `setup/` folder of
its own, because the digest lists the job's setup under that name.

A bare adapter, `opencode.json`, is the smallest configured harness: an adapter
alone. It is read exactly as a directory holding only `adapter.json` would be,
so every job and every `GUSTO_HARNESSES` directory written before this still
runs, and a harness moves from one form to the other without changing what it
is. A place that offers both `opencode.json` and `opencode/` is refused rather
than guessed at.

### Its digest

A configured harness is named by its **digest**, a SHA-256 over its files and
its setup's,
because a name is what two teams can both say while meaning different things.
Anybody holding the directory computes the same string, with or without gusto;
[FORMATS.md](FORMATS.md#the-configured-harness-digest) gives the steps. Every run
records the digest of the configured harness that ran, and two runs are the
same harness's only when their digests match.

### Where it lives

A job carries the configured harness it runs in `harnesses/`: `gusto new` copies
it from `GUSTO_HARNESSES`, the whole directory as it copies a bare adapter, and
the runner takes the digest from the job's copy. A harness built into gusto is
the one exception: every runner already holds it, so the job carries nothing
and the digest is of the adapter built into the runner that ran it.

There is no registry. A configured harness is shared as a git repository or a
tarball, and the digest leaves out what those two differ in, so both give the
same digest.

### What it declares and what the run proves

Skills, the prompt, the model policy and the adapter are files, and the
directory and the setup hold them. The harness program itself, an MCP server's
process and the model are referenced rather than held: a program's name in the
adapter or in `servers.json`, resolved when the job runs. A digest over a reference is not proof
of what the reference resolved to. The directory declares; the evidence of a
run is what proves, and recording what each reference resolved to beside the
digest is work still to come.

### What it may not change

A configured harness is read, not written. That holds for what the job carries;
a harness given skills finds them where its adapter's `skills` says, which is
outside the job and outside the digest, and the runner copies them there
afresh from the setup before each run. The digest is taken before the
harness starts, so nothing it writes during a run changes what the manifest
says ran, and a harness that does write to its own directory shows up as a
changed harness when the job is verified. A harness that learns writes what it
learned to the job's `out/`, where it is evidence of that run, not a quiet change
to the next one.

## Why the distinction decides the format

`job.json` names a harness, and the name finds a configured harness: in the
job, in `GUSTO_HARNESSES`, or built in. The manifest then names what the name
found, by digest. A marketplace, a rented harness, or bring-your-own all trade
in the configured one, so one day that field will name a digest to fetch, the
way you run an image rather than a binary name.

That is the change to make once something genuinely needs fetching by digest.
Designing the registry before anything needs registering is how this stops being
a product.
