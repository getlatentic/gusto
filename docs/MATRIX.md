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
  say where it ran cannot be read (#44).

[`matrix/issue-8.json`](matrix/issue-8.json) is the two local rows from #8, and
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
