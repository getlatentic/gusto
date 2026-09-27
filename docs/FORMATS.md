# The formats

Two files are the contract between the parts of a run, and between haas and
anybody who writes their own runner or reader: `job.json`, which says what to
do, and `out/manifest.json`, which records what happened. Each is published as a
JSON Schema, draft 2020-12.

| file | schema | `$id` |
| --- | --- | --- |
| `job.json` | [`schema/job/v1.json`](../schema/job/v1.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/job/v1.json` |
| `out/manifest.json` | [`schema/manifest/v1.json`](../schema/manifest/v1.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/manifest/v1.json` |
| `audit.json` | [`schema/audit/v2.json`](../schema/audit/v2.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/audit/v2.json` |
| `out/manifest.sig` | [`schema/signature/v1.json`](../schema/signature/v1.json) | `https://raw.githubusercontent.com/getlatentic/haas/main/schema/signature/v1.json` |

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

## The signature

`out/manifest.sig` is a detached Ed25519 signature over the exact bytes of
`out/manifest.json`. It is detached because a signature inside the file it signs
would have to be taken out again before checking, and the bytes left would then
need one canonical JSON form that every writer and reader agrees on. Signing the
bytes on disk needs no such form, and any Ed25519 tool can check it: `openssl
pkeyutl -verify -pubin -inkey <name>.pub -rawin -in manifest.json -sigfile <the
decoded signature>` does. The code that writes and checks it is
`internal/signing` and `internal/evidence`.

| field | holds |
| --- | --- |
| `scheme` | `ed25519` |
| `key` | the key id: `sha256:` and the first 16 bytes of the SHA-256 of the raw public key, in hex |
| `publicKey` | the raw 32-byte public key, base64 |
| `signature` | the 64-byte signature, base64 |

The public key it carries lets `evidence.Check` tell that a manifest changed
after it was signed, with no keyring, so `verify`, `export`, `replay` and `check`
all refuse one. It says nothing about who signed: any key vouches for itself.
Who signed is the verifier's keyring's answer, a directory of `<name>.pub` files
named by `HAAS_KEYRING`, looked up by `key` and checked against the keyring's own
copy of the key. The manifest's `signer` field stays what it was, the runner's
own word for itself, and nothing shows it as the signer: `haas show`, the pull
request `haas ship` opens and `audit.json` each name the signer through
`evidence.SignedBy`, which gives the keyring's name or says in words why it
cannot (`unsigned`, `signed, not checked: no keyring`, `signed, not checked: key
<id> is not in the keyring`, `signed, and the signature does not hold`).

A record without `manifest.sig` is unsigned. `haas verify` says so in those
words and passes it, unless `-signed` is given, because every record made before
signing is unsigned and the version rule below promises it still verifies. A
signature from a key the keyring does not hold is refused by the key's id.

### Why signing keeps the manifest at version 1

Signing adds a file and changes no field of the manifest. An unsigned manifest
is byte for byte what it was, and a signed one has the same fields meaning the
same things. `files` leaves `manifest.sig` out for the reason it leaves
`manifest.json` out: the signature is made over the manifest after it is
written, so the manifest cannot hold its hash. That is an additive change, so
the version stays.

What an older reader does with a signed record is the test of that. It hashes
`out/` without knowing the new file, and reports `manifest.sig is not in the
manifest`. That is a refusal by name, never a silent misreading, which is the
failure this document asks a reader to prefer. Remove the signature and the
older reader verifies the record as what the signature's absence makes it:
unsigned.

## The audit export

`haas export` writes a third format, `audit.json`, at the root of an export
directory. It follows the same version rules as the other two, and its schema is
[`schema/audit/v2.json`](../schema/audit/v2.json). The code that writes and
checks it is `internal/audit`.

### Why the signer raised it to version 2

In version 1, `evidence.signer` copied the manifest's `signer` field: the
runner's claim, checked by nothing. In version 2 it is who the exporter's
keyring says signed `out/manifest.json`, or the words above for a record no
keyring vouched for. The field kept its name and type and changed what it means,
which is the change this document says raises the version, so version 2 is
published beside [`schema/audit/v1.json`](../schema/audit/v1.json) rather than
over it.

A reader of this build still reads a version 1 export, as version 1: it derives
the summary again with the signer taken from the manifest, which is what version
1 wrote there, and so an old export verifies as what it said and vouches for no
signer. An older reader refuses a version 2 export by name.

The signer is the one value in `audit.json` that rests on something outside the
export: the keyring. `haas export -verify` names it again with its own
`HAAS_KEYRING`, and when that answer differs from the one the export wrote, it
reports both, naming `evidence.signer`, rather than choosing which to believe.

### What an export is

An export is a job directory without its workspace: `job.json` and `out/` exactly
as the run left them, plus `state.json` and the request lines when there are
some, plus `audit.json`. Keeping that shape is what lets the manifest check run
on an export unchanged. `internal/audit` calls `evidence.Check` for it rather than
hashing files itself, so whatever that check comes to hold, such as a signature
on the manifest, an export is held to as well.

### Where each answer comes from

| question | field | from |
| --- | --- | --- |
| who asked | `asked` | `state.json`: how the job arrived and when; the person is `not recorded` |
| what ran | `ran` | `job.json` as written, and the harness, its exit and the budget from the manifest |
| what it decided | `outcome` | the manifest's gate, and `state.json`'s state and times |
| what changed | `changed` | `out/diff.patch`, with the hash the manifest holds for it |
| which model | `model` | `job.json`'s model, the manifest's endpoint, the credential names in `out/proxy.jsonl` |
| how much | `spend` | the harness's reported tokens and cost from the manifest; the calls, bytes, tokens and cost of the model calls `out/proxy.jsonl` records as sent; and the calls a spending cap refused, which are counted apart |
| where else | `outbound` | the hosts other than the model in `out/proxy.jsonl`, reached and denied |
| who approved | `approval` | `not recorded` |
| who looked | `requests` | the request log's lines whose path names the job |
| who signed | `evidence.signer` | `out/manifest.sig`, checked against the exporter's `HAAS_KEYRING`; never the manifest's `signer` field |

The request that created a job is never among its request lines. It is `POST
/instances/{id}/tasks`, a thread's `POST .../say` or `POST /api/jobs`, and the
job's ID is chosen after that path is written, so the path is the only tie and
it does not hold one. What the lines show is who read the job afterwards.

### The words in place of a value

A value the export does not have is one of four strings in its place, never a
missing field and never a guess:

| word | means |
| --- | --- |
| `not recorded` | the platform does not write this down, or this job never had it |
| `redacted` | the export's policy dropped the file it comes from, or replaced the value in it |
| `none` | recorded as not happening: no gate for a turn in a conversation, no session for a one-shot job |
| `not measured` | the proxy saw the calls but not what they cost: an answer carried no usage, or the model has no price |

`spend.tokens` is `not recorded` where the manifest has `null`, for the same
reason the manifest has `null`: nothing reported is nothing claimed.

### Redaction, and why it does not break verification

A policy drops files and replaces values, so a redacted file no longer matches
its hash in the manifest. The decision is to expect exactly that and nothing
more. Every file the policy touched is listed in `redactions` with the rules that
touched it and the hash the manifest holds for the original. Verification then
checks three things:

1. `evidence.Check` on the export. A dropped file may be reported missing and a
   rewritten one changed; every other complaint fails the export, naming its
   file. A redaction whose original hash is not the one the manifest holds fails
   too, so a redacted file cannot be tied to somebody else's run.
2. Every file against `carried`, the hash the export recorded for it. This holds
   what the manifest cannot: `state.json`, the request lines, and each redacted
   copy.
3. `audit.json` against a summary derived again from the files, one section at a
   time, so a summary that says more than its files fails by section.

What a redacted file loses is the run's word for it. Its `carried` entry has
`covered: false`, and the only way to tie it to the run is to hold the original
and match the hash in `redactions`. Everything the policy left alone keeps
`covered: true` and verifies against the run's own manifest.

`job.json` and `out/manifest.json` cannot be dropped, because the check starts
from them. The manifest is never rewritten either: it holds names and hashes and
nothing to hide. `job.json`'s values can be redacted, and redacting `brief` also
redacts the copy of it in `state.json`, because a field rule applies to every
JSON file the export carries.

### Retention

Retention here is what one export keeps: the `drop` list, as slash paths inside
the export or `path.Match` patterns. How long a server keeps job directories is a
different question and nothing decides it yet.
