# Contributing

**Issues are the single source of truth, and they live in
[getlatentic/planning](https://github.com/getlatentic/planning)**, the private
tracker for every Latentic product. This repository has no issues of its own.
No work happens that is not an issue there, and every change references one.

- Every haas issue carries the `repo:haas` label. Milestones are named
  `haas · <goal>` and are sprints.
- Branch per issue. The pull request says `Closes getlatentic/planning#N`, or
  `Refs getlatentic/planning#N` when it only moves part of it. A bare `#N` points
  at this repository and finds nothing.
- Tick the sub-tasks in the issue as you go, and close it when its acceptance
  criteria are met.
- Work that surfaces mid-sprint goes to the backlog with the `backlog` label and
  no milestone. It joins a sprint at sprint planning, not the moment it is
  thought of.
- Every issue belongs to a sprint or to the backlog.

## The standard for the code

- A number in a README or a comment comes from a run somebody did. No estimates
  dressed as measurements.
- An adapter's field paths are read off a real run of that harness. A test holds
  the line that was observed.
- One cohesive concern a file. Split at about 400 lines rather than later.
- A comment says why, and describes the present. It does not narrate a change.

## Before a pull request

```bash
gofmt -l .
go vet ./...
go test ./...
```
