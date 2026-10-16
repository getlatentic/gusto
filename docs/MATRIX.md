# Which pairs finish a job

`gusto matrix` (`internal/matrix`) runs a fixed set of jobs on every harness and model pair a plan names, and keeps every run. It answers whether a harness with a given model can finish the job at all, and what finding out costs.

```bash
gusto new one-line-fix -repo /path/to/repo -brief "fix the operator" -- sh test.sh
gusto matrix plan.json -here     # run every cell into plan-matrix/, print the table
gusto matrix plan-matrix         # print the table from the evidence alone
gusto matrix plan-matrix -json   # the same table, as the catalog reads it
```

## The plan

```json
{
  "environment": "laptop",
  "jobs": ["one-line-fix"],
  "simplest": "one-line-fix",
  "pairs": [
    { "harness": "opencode", "model": "ollama/gpt-oss:20b" },
    { "harness": "opencode", "model": "ollama/llama3.2:3b" }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `jobs` | job directories as `gusto new` leaves them, relative to the plan |
| `pairs` | a harness and its model. With no model, the pair uses the job's model or the harness's default |
| `simplest` | the job a pair must finish before it is worth offering. The first job when left out |
| `environment` | where it ran. The same pair cost twenty times as much on a laptop, which carries its whole skill set into every request, as in a container that starts empty (getlatentic/planning#270) |

Example plans are in [`matrix/`](matrix/): `issue-8.json` (two local rows), `box.json` (thirteen pairs on the box's three credentials), `gemma4.json`, `flash.json` and `gemma-translated.json`.

## Layout and table

`plan-matrix/` holds `matrix.json` (where it ran and how long each cell took), `table.md` for a README, `table.json` for the catalog, and one directory for each job. A job directory is a comparison as `gusto compare` writes it: `comparison.json` above one ordinary job directory for each pair, such as `opencode@ollama-gpt-oss-20b/`. Each comparison is pinned once to one commit ([COMPARE.md](COMPARE.md)). `gusto compare plan-matrix/one-line-fix` prints any column. Every run verifies, ships and replays like any other job.

| Column | Comes from |
| --- | --- |
| finished | the gate's verdict in the manifest. A run with none is "did not run" |
| wall clock | measured around the run: clone, container start, harness and gate |
| billed tokens, cost | `out/proxy.jsonl` when the run went through the proxy, otherwise the harness's own report. The column says which |
| evidence | "checks", or the first file that changed since the run |

A number the run did not leave reads "unmeasured", never zero. A call the proxy could not price makes the cost "at least".

## Reruns, environment and exit code

- A cell with a verdict is not run again. `-again all` or `-again opencode@ollama/gpt-oss:20b,codex@deepseek/deepseek-flash` reruns cells, and the replaced run is kept as `.attempt-1`.
- A cell with no verdict, because its program was missing or the machine stopped, runs again next time. A pair added later runs alone, at the commit the job was first pinned to.
- Cells run one after another, so runs do not slow each other.
- `-here` runs every cell in this process. Without it each cell runs in a container, through the proxy when a credential is held (`GUSTO_CREDENTIALS` or `GUSTO_MODEL_UPSTREAM`), otherwise at `GUSTO_MODEL_URL`.
- A matrix refuses to extend from another environment. Use `-into` for a new matrix.
- Exit 0 when every cell left a verdict and its evidence checks, whichever pairs finished. Otherwise exit 2.

`GUSTO_MATRIX` names a matrix's `table.json` for `gusto serve` to read at start. A pair that ran the simplest job, left a verdict and never finished it is marked, and the catalog's `CannotFinish(model)` names the job, environment and run. An unmeasured pair is not marked. A table that cannot be read is logged and skipped.

## Results

All runs used the one-line-fix job in an OpenShell sandbox on one `m8i.xlarge`. Low-priced Bedrock models, through Mantle (plan: [`matrix/gemma4.json`](matrix/gemma4.json)). Gemma 4 has its own credential, `gemma`, whose address ends in `/openai`:

| Model | `opencode` | `pi` | `hermes` | `dsh` | `codex` |
| --- | --- | --- | --- | --- | --- |
| `google.gemma-4-26b-a4b` | yes, 52 s | yes, 27 s | yes, 41 s | no | no |
| `google.gemma-4-31b` | yes, 2 m 46 s | yes, 29 s | yes, 41 s | no | no |
| `google.gemma-4-e2b` | no | yes, 33 s | no | no | no |
| `openai.gpt-oss-120b` | not tried again | yes, 42 s | yes, 43 s | no | no |

- `opencode` runs only models its configuration declares (`internal/harness/config/opencode/opencode.json`). It finishes on the 26B and 31B models. On `gemma-4-e2b` the tests still fail.
- `dsh` asked `gpt-oss-120b` and `gemma-4-e2b` for 256,000 output tokens, which they do not allow. The other two models answered it with no content.
- `codex` makes one model call and then has its requests denied, on every model here. It finished on DeepSeek Flash and Pro through DeepSeek's own API.
- `hermes` ends without a fix on `gemma-4-e2b`.

Prices from [aws.amazon.com/bedrock/pricing](https://aws.amazon.com/bedrock/pricing/) on 2026-10-01 are in `deploy/aws/openshell/ansible/roles/demo/files/spend-policy.json`. A new chat starts on `claude` with `google.gemma-4-26b-a4b` (`GUSTO_CHAT_DEFAULT`). Per million tokens, input and output: `google.gemma-4-26b-a4b` $0.13 and $0.40, `google.gemma-4-31b` $0.14 and $0.40, `google.gemma-4-e2b` $0.04 and $0.08.

DeepSeek Flash on every harness, on 2026-10-02 (plan: [`matrix/flash.json`](matrix/flash.json)). `claude` uses `https://api.deepseek.com/anthropic`, the others `https://api.deepseek.com`. All finished. Cost read "unmeasured" for `codex`, `claude` and `dsh`, and the host's spend policy had no DeepSeek prices. Wall clock and model calls: `opencode` 2 m 46 s and 8, `codex` 35 s and 6, `claude` 30 s and 5, `pi` 28 s and 6, `dsh` 39 s and 7, `hermes` 40 s and 8.

Claude Code and Codex on Gemma, translated (2026-10-05, plan: [`matrix/gemma-translated.json`](matrix/gemma-translated.json)). The Gemma models answer Chat Completions alone, so the proxy translates the Messages and Responses calls ([GATEWAY.md](GATEWAY.md)). The `gemma` credential sets `"wires": ["chat"]` and `"maxOutput": 8192`. All five finished. Wall clock and model calls: `claude` on 26b 33 s and 8, `claude` on 31b 40 s and 8, `codex` on 26b 1 m 13 s and 41, `codex` on 31b 35 s and 11, and `opencode`, which is not translated, on 26b 38 s and 10.
