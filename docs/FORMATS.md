# The formats

Two files are the contract between the parts of a run, and between gusto and anyone who writes a runner or reader: `job.json` says what to do, and `out/manifest.json` records what happened. Each has a JSON Schema, draft 2020-12. The tests validate against these files, embedded into the build. The job that `gusto new` writes, and the job and manifest a fixture run leaves, must match their schema.

| File | Schema | `$id` |
| --- | --- | --- |
| `job.json` | [`schema/job/v1.json`](../schema/job/v1.json) | `https://raw.githubusercontent.com/getlatentic/gusto/main/schema/job/v1.json` |
| `out/manifest.json` | [`schema/manifest/v1.json`](../schema/manifest/v1.json) | `https://raw.githubusercontent.com/getlatentic/gusto/main/schema/manifest/v1.json` |
| `audit.json` | [`schema/audit/v2.json`](../schema/audit/v2.json) | `https://raw.githubusercontent.com/getlatentic/gusto/main/schema/audit/v2.json` |
| `out/manifest.sig` | [`schema/signature/v1.json`](../schema/signature/v1.json) | `https://raw.githubusercontent.com/getlatentic/gusto/main/schema/signature/v1.json` |

## Versions

Each file carries `"version": 1`. A file without one is read as version 1.

| Change | Version |
| --- | --- |
| A new optional field | Same version. The schema file is updated in place |
| Renaming a field, changing its type, unit or default, or making an optional field required | New version, published as `v2.json` beside the old file |

A reader ignores a field it does not know. It refuses a version newer than it knows, naming the file and both versions, because a field whose meaning changed would be obeyed wrongly. The schemas close every object with `"additionalProperties": false`, so a field a writer adds without describing it fails the tests. To validate a file from a newer writer of the same version, use the schema at `main`.

## The signature

`out/manifest.sig` is a detached Ed25519 signature over the exact bytes of `out/manifest.json`. Any Ed25519 tool can check it (code: `internal/signing`, `internal/evidence`). With openssl:

```sh
openssl pkeyutl -verify -pubin -inkey <name>.pub -rawin -in manifest.json -sigfile <the decoded signature>
```

| Field | Holds |
| --- | --- |
| `scheme` | `ed25519` |
| `key` | The key id: `sha256:` and the first 16 bytes of the SHA-256 of the raw public key, in hex |
| `publicKey` | The raw 32-byte public key, base64 |
| `signature` | The 64-byte signature, base64 |

- The carried public key lets `evidence.Check` tell that a manifest changed after signing, with no keyring. `verify`, `export`, `replay` and `check` all refuse a changed one.
- The signature does not say who signed, because any key vouches for itself. The verifier's keyring does: a directory of `<name>.pub` files named by `GUSTO_KEYRING`, looked up by `key` and checked against the keyring's copy.
- The manifest's `signer` field is the runner's own word. `gusto show`, the job page, the `gusto ship` pull request and `audit.json` name the signer through `evidence.SignedBy`, which gives the keyring's name or one of: `unsigned`, `signed, not checked: no keyring`, `signed, not checked: key <id> is not in the keyring`, `signed, and the signature does not hold`.
- A record without `manifest.sig` is unsigned. `gusto verify` says so and passes it, unless `-signed` is given. A signature from a key the keyring does not hold is refused by the key's id.
- The signature adds a file and changes no manifest field, so the manifest stays at version 1. `files` leaves out `manifest.sig` because the signature is made after the manifest is written. An older reader hashes `out/` and reports `manifest.sig is not in the manifest`. Remove the signature and it verifies the record as unsigned.

## The configured harness digest

`harnessDigest` in `out/manifest.json` names the configured harness that ran (see [TERMS.md](TERMS.md#configured-harness)). It is `sha256:` and 64 lowercase hex digits, computed from the harness's files alone. A clone and a tarball give the same string. The code is `internal/harness/digest.go`.

1. Take every regular file under the harness's directory, by its path relative to that directory, with `/` between the parts.
   - A bare adapter, `name.json`, is one file at the path `adapter.json`.
   - When the job carries a setup, every file under its `setup/` joins them at `setup/` and its path there. A harness directory with its own `setup/` is refused.
   - In `setup/servers.json`, replace the value of every `env` and `headers` entry with an empty string, keep the name, and write the list as compact JSON with sorted keys, as Go's `encoding/json` writes it. Rotating a token leaves the digest as it was.
2. Leave out any file or directory named `.git`, `.hg` or `.svn`, and everything under it.
3. Refuse what is not a file. A symbolic link, device, socket, pipe, or a path with a line break fails the digest with its path named. A directory with no files adds nothing.
4. Sort the paths byte for byte, as `LC_ALL=C sort` does.
5. Write one line a file: its mode, a space, the SHA-256 of its content in lowercase hex, a space, its path, and a newline. The mode is `100755` when any execute bit is set, and `100644` otherwise.
6. The digest is `sha256:` and the lowercase hex SHA-256 of all the lines together.

The digest leaves out timestamps, owner and group, and read and write bits, because a clone, a tarball and a copy into a job each set them afresh.

In a POSIX shell with GNU `find`, `sort` and `sha256sum`, run in the harness's directory with the job's `setup/` copied into it:

```sh
find . \( -name .git -o -name .hg -o -name .svn \) -prune -o -type f -printf '%P\n' |
  LC_ALL=C sort | while IFS= read -r path; do
    if [ -n "$(find "$path" -perm /111)" ]; then mode=100755; else mode=100644; fi
    printf '%s %s %s\n' "$mode" "$(sha256sum < "$path" | cut -c1-64)" "$path"
  done | sha256sum | cut -c1-64
```

This prints the hex without the prefix and does not refuse links, so run `find . -type l` first. `internal/harness/digest_test.go` holds a fixture's digest, and these shell steps give the same one. `setup_test.go` holds that a harness and its setup digest as one directory.

- The runner reads the configured harness once, before the harness starts. It takes the adapter and the digest from that one reading, from the job's `harnesses/` and `setup/`. A harness built into gusto is not carried. Its digest is of the adapter built into the runner.
- A harness that writes to its own directory in the job changes the copy the job carries. `evidence.Check` digests the copy again and reports `harnesses/<name> changed`, and `gusto verify` fails the record. A job that carries no harness, and an export, have nothing to check.
- Two runs with the same `harnessDigest` ran the same configured harness. `evidence.Comparable` says which it is: the same configuration, the same name configured differently, different harnesses, or a run with no digest.
- `harnessDigest` is a new optional field, so the manifest stays at version 1. A manifest written before it reports `no digest recorded`, and is comparable by name only.

## The audit export

`gusto export` writes `audit.json` at the root of an export directory. It follows the same version rules. Its schema is [`schema/audit/v2.json`](../schema/audit/v2.json), and the code is `internal/audit`.

Version 2 changed `evidence.signer`. Version 1 copied the manifest's `signer`, the runner's unchecked claim. Version 2 is who the exporter's keyring says signed `out/manifest.json`, or one of the words above. A reader of this build still reads a version 1 export, deriving the summary with the signer from the manifest. An older reader refuses a version 2 export by name. [`schema/audit/v1.json`](../schema/audit/v1.json) stays beside v2. `gusto export -verify` names the signer again with its own `GUSTO_KEYRING`. When the answer differs from the export's, it reports both, naming `evidence.signer`.

An export is a job directory without its workspace: `job.json` and `out/` as the run left them, plus `state.json`, the request lines when there are any, and `audit.json`. `internal/audit` calls `evidence.Check`, so any check that gains a rule, such as a manifest signature, holds for exports too.

| Question | Field | From |
| --- | --- | --- |
| who asked | `asked` | `state.json`: how the job arrived and when. The person is `not recorded` |
| what ran | `ran` | `job.json` as written, and the harness, its exit and the budget from the manifest |
| what it decided | `outcome` | The manifest's gate, and `state.json`'s state and times |
| what changed | `changed` | `out/diff.patch`, with the hash the manifest holds |
| which model | `model` | `job.json`'s model, the manifest's endpoint, the credential names in `out/proxy.jsonl` |
| how much | `spend` | The harness's reported tokens and cost from the manifest. The calls, bytes, tokens and cost of model calls `out/proxy.jsonl` records as sent. The calls a spending cap refused, counted apart |
| where else | `outbound` | The hosts other than the model in `out/proxy.jsonl`, reached and denied |
| who approved | `approval` | `not recorded` |
| who looked | `requests` | The request log lines whose path names the job |
| who signed | `evidence.signer` | `out/manifest.sig`, checked against the exporter's `GUSTO_KEYRING`, never the manifest's `signer` field |

The request that created a job is not among its request lines, because the job's ID is chosen after that path is written.

A value the export does not have is one of four words, never a missing field and never a guess. `spend.tokens` is `not recorded` where the manifest has `null`.

| Word | Means |
| --- | --- |
| `not recorded` | The platform does not write this down, or this job never had it |
| `redacted` | The export's policy dropped the file it comes from, or replaced the value |
| `none` | Recorded as not happening: no gate for a turn in a conversation, no session for a one-shot job |
| `not measured` | The proxy saw the calls but not their cost: no usage in an answer, or no price for the model |

### Redaction and verification

A policy drops files and replaces values, so a redacted file no longer matches its hash in the manifest. `redactions` lists every file the policy touched, with the rules and the hash the manifest holds for the original. Verification checks three things:

1. `evidence.Check` on the export. A dropped file may be reported missing and a rewritten one changed. Every other complaint fails the export, naming its file. A redaction whose original hash is not the manifest's fails too.
2. Every file against `carried`, the hash the export recorded for it. This covers `state.json`, the request lines and each redacted copy.
3. `audit.json` against a summary derived again from the files, one section at a time.

A redacted file has `covered: false` in `carried`. Tie it to the run by holding the original and matching the hash in `redactions`. Everything else has `covered: true` and verifies against the run's manifest. `job.json` and `out/manifest.json` cannot be dropped. The manifest is never rewritten. `job.json` values can be redacted, and redacting `brief` also redacts its copy in `state.json`, because a field rule applies to every JSON file the export carries. Retention for an export is its `drop` list: slash paths inside the export, or `path.Match` patterns. How long a server keeps job directories is not decided.
