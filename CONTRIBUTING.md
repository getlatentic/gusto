# Contributing

Thank you for looking. gusto is the program behind gusto, under the Apache License 2.0 ([LICENSE](LICENSE)). A
contribution is licensed the same way.

## Start with an issue

Open an issue in this repository before a large change, so we can say whether it fits before you spend a week on it. A bug report
with what you ran, what happened and what you expected is the most useful thing you can send. For a vulnerability, do not open an
issue: [SECURITY.md](SECURITY.md) says where to send it.

[docs/DIRECTION.md](docs/DIRECTION.md) says what gusto is for and is not yet. [docs/PROTOCOL.md](docs/PROTOCOL.md) is the calls a client makes
to a server, and the place to start if you are building against it.

## The standard for the code

- A number in a README or a comment comes from a run somebody did. No estimates dressed as measurements.
- An adapter's field paths are read off a real run of that agent. A test holds the line that was observed.
- One cohesive concern a file. Split at about 400 lines rather than later.
- A comment says why, and describes the present. It does not narrate a change.
- A change comes with the test that fails without it.

## Before a pull request

```bash
gofmt -l .        # prints nothing
go vet ./...
go test ./...
```

On a Mac, a build that uses cgo needs `CC=/usr/bin/clang` when a different `clang` is first on your PATH.

Say in the pull request what changed, what you ran to check it, and what you did not check. One concern a pull request. Reference the
issue it answers with `Closes #N`, or `Refs #N` when it only moves part of it.
