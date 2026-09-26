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

**Nothing here holds one yet.** The germ is there — an adapter already travels
in a job — but an adapter is the run recipe, not the configuration.

## Why the distinction decides the format

`job.json` names a harness, so today it names a program. A marketplace, a rented
harness, or bring-your-own all trade in the configured one, so that field will
have to name a **package** with the program inside it, the way you run an image
rather than a binary name.

That is the change to make once a second kind of harness genuinely needs it.
Designing the registry before anything needs registering is how this stops being
a product.
