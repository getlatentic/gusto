# Comparing harnesses

`gusto compare` (`internal/compare`) runs one job on several harnesses and keeps the runs together as one comparison. A team can then find out which agent is better on its own code instead of trusting a benchmark.

```bash
gusto new /tmp/fix -repo /path/to/repo -brief "make the answer yes" -- go test ./...
gusto compare /tmp/fix claude opencode     # make /tmp/fix-compare-1, run both, print them
gusto compare /tmp/fix-compare-1           # print it again, from the evidence
```

## What a comparison is

```
fix-compare-1/
  comparison.json   the submission, the commit, what is held, and the runs
  claude/           an ordinary job directory
  opencode/         an ordinary job directory
```

Each run is a whole job with its own `job.json`, `workspace/` and `out/`, made from the submission's `job.json` with the harness changed. It verifies, ships and replays like any other job. The comparison moves as one directory, because `comparison.json` names the runs relative to itself.

## What is held constant

The runs differ in the harness and in nothing else the job controls. `comparison.json` lists the copied fields in `held`, read off the files.

| Held | How |
| --- | --- |
| the brief, the repository, the sensitivity | copied |
| the gate | copied. It is the only thing that gives a verdict, so it must be the same command |
| the budget | copied, turns and seconds |
| the commit | pinned once, before any run clones, and written into every run's `ref` |
| the model | copied, when the submission names one |

The commit is pinned as `gusto replay` pins one: the submission's `ref` is resolved in a throwaway clone against `refs/remotes/origin/*`, then `refs/tags/*`, and a commit id is its own answer. A branch that moves between two clones therefore cannot hand the harnesses different code.

A harness that signs in to its own account uses that account's model. When the submission names no model, `held` does not list it, and the comparison is of harness and model together. Name a model to compare harnesses alone.

Not held:

- The adapter. It is the harness.
- The hour. The runs are queued together and start together when runners are free and no person limit ([LIMITS.md](LIMITS.md)) holds one back.
- The model's answers. Two runs of one harness still differ, so one comparison is one sample.

## Isolation

In a container, the default, each run mounts only its own directory. A harness cannot read the other run's workspace, evidence or `comparison.json`. `-here` runs both in this process with separate directories and no other barrier. Use it only for harnesses you trust, such as a fixture.

## Exit code

`gusto compare` exits 0 when every run ran, left a verdict and its evidence still checks, whichever harness did better. Otherwise it exits 2.

## From the pages

`/compare` gives one task to several engines on a template that has a check. Today that is the Software engineer template, checked by `node --test`.

- Each run is an ordinary job in the queue. It shows in Activity and Spending and keeps its own signed record.
- Every run clones the template's sample from one commit, held in `GUSTO_COMPARISONS/samples/<template>`, with the same model, skills and check.
- `GUSTO_COMPARISONS/<id>.json` names the runs by job id.
- `/compare/<id>` shows each run's verdict, time, cost, model calls and change size, and links each signed record. Once every run has finished, it says in one sentence which did better.

## What the sentence says

- Only runs that passed the check are ranked. When one run passes alone, the sentence says so and names no fastest or cheapest.
- Two figures are level when they differ by less than 15% of the larger, or by less than 5 seconds, or by less than a tenth of a cent.
- The best two must stand apart. A level pair at the front gets no badge, and the sentence names the two. When nothing separates the runs, the sentence says that.
- A cost the proxy could not price in full is a floor, "at least". A floor can show a run is not the cheapest. It can never make one the cheapest.
- A run with no time, or no cost counted at all, is not ranked.
- Model calls are the turns of the agent's loop, counted at the proxy from the provider's answers. A run that did not use the proxy shows none.

## Not yet built

The table shows the verdict, the harness's exit, the budget, tokens and cost. To compare patches, read the two `diff.patch` files.
