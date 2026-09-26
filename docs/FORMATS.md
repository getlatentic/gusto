# The formats

Two files are the contract between the parts of a run, and between haas and
anybody who writes their own runner or reader: `job.json`, which says what to
do, and `out/manifest.json`, which records what happened. Each is published as a
JSON Schema, draft 2020-12.

| file | schema | `$id` |
| --- | --- | --- |
| `job.json` | [`schema/job/v1.json`](../schema/job/v1.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/job/v1.json` |
| `out/manifest.json` | [`schema/manifest/v1.json`](../schema/manifest/v1.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/manifest/v1.json` |

The tests validate against these same files, embedded into the build. A job that
`haas new` writes and the job and manifest a fixture run leaves must each match
their schema, so a writer that drifts from the published format fails a test.

## The version field

Each file carries `"version": 1`. Every writer puts it in. A file without one
predates the field and is read as version 1, so every job directory made before
it still runs and still verifies.

## What changes the version

- **An additive change keeps it.** A new optional field joins the schema at the
  same version, and the schema file is updated in place.
- **A change to what a field means raises it.** Renaming a field, changing its
  type or its unit, changing a default, or making an optional field required
  each publish a new file, `v2.json`, beside the old one.

## What a reader does

- **A field it does not know**, it ignores. That is what lets an old runner read
  a job written after an additive change.
- **A version newer than it knows**, it refuses by name, with the file and both
  versions in the message. It does not read the file as the version it knows,
  because a field whose meaning changed would be obeyed wrongly and without a
  word.

The schemas themselves are stricter than a reader: they close every object with
`"additionalProperties": false`. The schema is the writer's contract, and a
field a writer adds without describing it should fail the tests. To validate a
file from a newer writer of the same version, use the schema at `main`, which
carries every field that version has gained.
