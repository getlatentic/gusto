# Comparing harnesses

A comparison runs one job on several harnesses and keeps the runs together as
one comparison, so a team can find out which agent is better on its own code
rather than trusting a benchmark. It is `haas compare`, in `internal/compare`.

```bash
haas new /tmp/fix -repo /path/to/repo -brief "make the answer yes" -- go test ./...
haas compare /tmp/fix claude opencode     # make /tmp/fix-compare-1, run both, print them
haas compare /tmp/fix-compare-1           # print it again, from the evidence
```

## What a comparison is

```
fix-compare-1/
  comparison.json   the submission, the commit, what is held, and the runs
  claude/           an ordinary job directory
  opencode/         an ordinary job directory
```

Each run is a whole job with its own `job.json`, `workspace/` and `out/`, made
from the submission's `job.json` with the harness changed. It verifies, ships
and replays like any other job. `comparison.json` sits above the runs, and the
runs are named in it relative to it, so the comparison moves as one directory.

## What is held constant

The runs differ in the harness and in nothing the job controls. Everything
else in `job.json` is copied from the submission, and `comparison.json` lists
the fields it copied in `held`, read off the files rather than promised.

| held | how |
| --- | --- |
| the brief | copied |
| the gate | copied. It is the only thing that gives a verdict, so it has to be the same command |
| the budget | copied, turns and seconds |
| the sensitivity | copied, so the same rule decides for every run |
| the repository | copied |
| the commit | pinned once, before any run clones, and written into every run's `ref` |
| the model | copied, when the submission names one |

The commit is pinned the way `haas replay` pins one: the submission's `ref` is
resolved against a clone's record of the remote (`refs/remotes/origin/*`, then
`refs/tags/*`, and a commit id is its own answer). It happens once, in a clone
that is thrown away, because each run resolving the ref for itself would let a
branch that moved between two clones hand the harnesses different code.

The model is held only when the submission names one. A harness that signs in
to its own account uses that account's model, and a harness left to choose its
own default is free to. When `model` is absent, `held` does not list it, and
the comparison is one of harness and model together. Name a model to compare
harnesses alone.

## What is not held

- **The adapter.** It is the harness, and the harness is what is compared.
- **The hour.** The runs start together, so each meets the same time of day and
  the same load on a shared model, and neither waits on the other. Two runs of
  one harness still differ, because a model does; one comparison is one sample.
- **The model's answers.** Nothing here makes a model deterministic.

## Neither run sees the other

Each run has its own directory and its own clone. In a container, which is the
default, the container mounts only its own run's directory, so a harness
cannot read the other run's workspace, its evidence, or `comparison.json`,
which names the other harness. `-here` runs both in this process: the
directories are still separate, and nothing more than that keeps one harness
out of the other's, so use it for harnesses you trust, such as a fixture.

## The exit code

`haas compare` exits 0 when every run ran, left a verdict and its evidence still
checks, whichever harness did better, and 2 otherwise. Which harness did better
is what the comparison prints, not a failure of the comparison.

## From the pages

`/compare` gives one task to several engines on a template that has a check.
Today that is the Software engineer template, checked by `node --test`.

- Each run is an ordinary job in the queue, so it shows in Activity and
  Spending and keeps its own signed record.
- Every run clones the template's sample from one commit, held in
  `HAAS_COMPARISONS/samples/<template>`, with the same model, skills and check.
- `HAAS_COMPARISONS/<id>.json` names the runs by job id.
- `/compare/<id>` shows each run's verdict, time, cost, model calls and
  change size, links each run's signed record, and says in one sentence which
  did better once every run has finished.

## What the sentence says

Only runs that passed the check are ranked. A run that failed is not fast or
cheap at anything, and when one run passes alone the sentence says so and
names no fastest or cheapest.

Two figures are level when they differ by less than 15% of the larger, or by
less than 5 seconds, or by less than a tenth of a cent. A level pair gets no
badge, and the sentence says the time or the cost was level. When nothing
separates the runs, it says that. One run each is one sample, and a second run
could reverse a small difference.

A cost that is not priced for every model call is not ranked, and neither is a
run with no time: the run left out may have been the best, so the sentence says
the figure could not be compared.

Model calls are the turns of the agent's loop, counted at the proxy from the
provider's own answers. A run that did not go through the proxy shows none.

## Not yet

- **More than a verdict side by side.** The table shows the verdict, the
  harness's exit, the budget, tokens and cost. Comparing the patches is reading
  the two `diff.patch` files.
