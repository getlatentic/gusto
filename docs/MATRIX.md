# Which pairs finish a job

A matrix runs a fixed set of jobs on every harness and model pair a plan names,
and keeps every run. It answers the question a platform lead has before
offering a combination to a customer: can this harness, thinking with this
model, finish the job at all, and what does it cost to find out. It is
`haas matrix`, in `internal/matrix`.

```bash
haas new one-line-fix -repo /path/to/repo -brief "fix the operator" -- sh test.sh
haas matrix plan.json -here     # run every cell into plan-matrix/, print the table
haas matrix plan.json -here     # again: nothing that finished runs twice
haas matrix plan-matrix         # print the table from the evidence alone
haas matrix plan-matrix -json   # the same table, as the catalog reads it
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

- **jobs** are job directories as `haas new` leaves them, relative to the plan.
- **pairs** are a harness and the model it is given. A pair with no model runs
  whatever model the job names, or the harness's own default.
- **simplest** is the job a pair has to finish before it is worth offering. It
  is the first job when the plan leaves it out.
- **environment** names the place. The same harness on the same model has cost
  twenty times as much on a laptop, which carries its whole skill set into
  every request, as in a container that starts empty, so a row that does not
  say where it ran cannot be read (getlatentic/planning#270).

[`matrix/issue-8.json`](matrix/issue-8.json) is the two local rows from getlatentic/planning#234, and
[`matrix/box.json`](matrix/box.json) the thirteen pairs the box's three credentials
make possible. A test reads the first, so it stays a plan the matrix accepts.

## What a matrix is

```
plan-matrix/
  matrix.json                     where it ran, and how long each cell took
  table.md                        the table, for a README
  table.json                      the table, for the catalog
  one-line-fix/                   a comparison, as `haas compare` writes one
    comparison.json
    opencode@ollama-gpt-oss-20b/  an ordinary job directory
    opencode@ollama-llama3.2-3b/  an ordinary job directory
```

A matrix is many comparisons. Each job is a directory in the shape
[COMPARE.md](COMPARE.md) describes: `comparison.json` above one job directory
a pair, made from the job's `job.json` with the harness and model set, and
pinned once to one commit the way `haas replay` pins one. So
`haas compare plan-matrix/one-line-fix` prints any column of the matrix, and
every run verifies, ships and replays like any other job. `held` lists the
fields that are the same in every run, so a model is held only when every
pair asked for the same one.

## The table

| column | where it comes from |
| --- | --- |
| finished | the gate's verdict in the run's manifest. A run that left none "did not run" |
| wall clock | measured around the run: the clone, the container starting, the harness and the gate |
| billed tokens, cost | the proxy's record of what the provider answered, `out/proxy.jsonl`, when the run went through the proxy; the harness's own report otherwise. The column says which |
| evidence | "checks", or the first file that changed since the run |

A number the run did not leave reads "unmeasured", never zero, because a zero
reads as free. A call the proxy could not price makes the cost a floor, and the
table says "at least".

## Each cell costs money

- A cell whose run left a verdict is never run again. `-again all`, or
  `-again opencode@ollama/gpt-oss:20b,codex@deepseek/deepseek-flash`, runs
  those again, and the run it replaces is kept beside it as `.attempt-1`.
- A cell that left no verdict, because its program was missing or the machine
  stopped, runs again the next time the plan does.
- A pair added to the plan later runs alone, at the commit the job was pinned
  to the first time.
- The cells run one after another. The wall clock is one of the answers, and
  two runs sharing a machine or a local model would each be measured slowing
  the other down.

## One environment a matrix

`-here` runs every cell in this process, which reads this machine's own
harness configuration. Without it every cell runs in a container, the way a
server runs a job: through the proxy when a credential is held
(`HAAS_CREDENTIALS`, or `HAAS_MODEL_UPSTREAM`), so the cost is the proxy's
count, and straight at `HAAS_MODEL_URL` when none is. A matrix that ran in one
environment refuses to be extended from another; measure the second into its
own matrix with `-into`.

## The catalog

`HAAS_MATRIX` names a matrix's `table.json` for `haas serve` to read when it
starts. A pair that ran the simplest job, left a verdict, and finished it in no
environment is marked: the catalog entry's `CannotFinish(model)` says which job,
where, and which run, so the claim can be checked. A pair nobody measured is not
marked, because not knowing is not failing. Without `HAAS_MATRIX` the catalog
is what it was, and a table it cannot read is logged and passed over.

## The exit code

`haas matrix` exits 0 when every cell left a verdict and its evidence checks,
whichever pairs finished, and 2 otherwise.

## Low-priced Bedrock models on the box

Each pair ran the one-line-fix job in an OpenShell sandbox on one `m8i.xlarge`, through the Bedrock endpoint (Mantle). Mantle
serves Gemma 4 under `/openai/v1` and the other models, `gpt-oss` among them, under `/v1`, so Gemma 4 has a credential of its
own, `gemma`, whose address ends in `/openai`. [`matrix/gemma4.json`](matrix/gemma4.json) is the plan for the pairs the demo offers.

| model | `opencode` | `pi` | `hermes` | `dsh` | `codex` |
| --- | --- | --- | --- | --- | --- |
| `google.gemma-4-26b-a4b` | yes, 52 s | yes, 27 s | yes, 41 s | no | no |
| `google.gemma-4-31b` | yes, 2 m 46 s | yes, 29 s | yes, 41 s | no | no |
| `google.gemma-4-e2b` | no | yes, 33 s | no | no | no |
| `openai.gpt-oss-120b` | not tried again | yes, 42 s | yes, 43 s | no | no |

What stopped each that did not finish:

- `opencode` runs only models its configuration declares. Before `image/harness/opencode/opencode.json` named the Gemma 4
  models it made no model call; with them declared it finishes on the 26B and 31B models. `gemma-4-e2b` ends the job with the
  tests still failing.
- `dsh` asked `gpt-oss-120b` and `gemma-4-e2b` for 256,000 output tokens, which they do not allow, and on the other two Gemma 4
  models was answered with a response with no content.
- `codex` makes one model call and then has its requests denied, on every model here. It finished the job on DeepSeek Flash and
  Pro through DeepSeek's own API, so what is wrong is how it meets the Bedrock endpoint, not the model.
- `hermes` ends the job without a fix on `gemma-4-e2b`.

The demo offers `google.gemma-4-26b-a4b` and `google.gemma-4-31b` to `opencode`, `pi` and `hermes`, and to `claude` and `codex` through the translation below. It offers DeepSeek Flash to `claude`, `codex` and `dsh`, each of which finished on it; `dsh` gets no Gemma 4 model. A model's price per million
tokens, from [aws.amazon.com/bedrock/pricing](https://aws.amazon.com/bedrock/pricing/) on 2026-10-01, is in
`deploy/aws/openshell/ansible/roles/demo/files/spend-policy.json`, so the organisation's spending page shows what a demo cost:

| model | input | output |
| --- | --- | --- |
| `google.gemma-4-26b-a4b` | $0.13 | $0.40 |
| `google.gemma-4-31b` | $0.14 | $0.40 |
| `google.gemma-4-e2b` | $0.04 | $0.08 |

For comparison, DeepSeek Flash is $0.15 and $0.60, and the Qwen model the demo offered before is $0.53 and $2.66.

## DeepSeek Flash on every harness

[`matrix/flash.json`](matrix/flash.json) runs the one-line-fix job on all six harnesses with `deepseek-flash`, in an OpenShell
sandbox on one `m8i.xlarge`, on 2026-10-02. `claude` reaches it through DeepSeek's Anthropic-shaped endpoint
(`https://api.deepseek.com/anthropic`), the others through `https://api.deepseek.com`. Every harness finished and the gate passed.

| harness | finished | wall clock | model calls |
| --- | --- | --- | --- |
| `opencode` | yes | 2 m 46 s | 8 |
| `codex` | yes | 35 s | 6 |
| `claude` | yes | 30 s | 5 |
| `pi` | yes | 28 s | 6 |
| `dsh` | yes | 39 s | 7 |
| `hermes` | yes | 40 s | 8 |

So `codex` and `dsh` finish on a model served by DeepSeek and not on the Bedrock models above. The cost column read "unmeasured"
for `codex`, `claude` and `dsh`, because those harnesses did not report tokens the proxy could price. The measured host's spend
policy had no DeepSeek prices either.

## Claude Code and Codex on Gemma, translated

The Gemma models on Bedrock answer Chat Completions alone. Claude Code calls in Anthropic's Messages API and Codex in the
Responses API, so the proxy translates their calls (docs/GATEWAY.md). [`matrix/gemma-translated.json`](matrix/gemma-translated.json)
ran the one-line-fix job in an OpenShell sandbox on one `m8i.xlarge` on 2026-10-05, with the `gemma` credential naming
`"wires": ["chat"]` and `"maxOutput": 8192`. OpenCode, which calls in Chat Completions, is the pair that is not translated.

| harness | model | finished | wall clock | model calls |
| --- | --- | --- | --- | --- |
| `claude` | `google.gemma-4-26b-a4b` | yes | 33 s | 8 |
| `claude` | `google.gemma-4-31b` | yes | 40 s | 8 |
| `codex` | `google.gemma-4-26b-a4b` | yes | 1 m 13 s | 41 |
| `codex` | `google.gemma-4-31b` | yes | 35 s | 11 |
| `opencode` | `google.gemma-4-26b-a4b` | yes | 38 s | 10 |

Codex on these models had failed when its calls went to Bedrock's own Responses endpoint, which refuses a tool type
Codex sends (the table above). Claude Code first ran 213 calls without finishing: Bedrock numbers the tool calls of every
answer from `call_0`, and Claude Code took a second call by an identifier it had answered as answered already. The
translation now gives each call an identifier of its own.
