# Roadmap

What is next for gusto, in the order we are doing it. Each line names the issue it is tracked in where there is one. A line moves out of here
when it ships, and into [RELEASES.md](RELEASES.md).

## Now: a release anyone can install

- A numbered release, v0.1.0-rc.2 first, with the program for six platforms, arm64 images, and the root file system and disk tools the virtual
  machine sandbox needs.
- A Homebrew formula (`deploy/homebrew/formula.sh` writes it from a release) and a public repository.
- A server of your own in one command (`deploy/install/server.sh`), checked on a real domain and on x86-64.
- A five-minute quickstart, checked from the release on clean machines.

## Next: more ways in

- An MCP server, so a person working in Claude, Codex or an editor can start a governed task without leaving it. It is a layer over
  [the protocol](PROTOCOL.md).
- Install and first-run counts, opt in and transparent, so we can see where people stop.

## Stronger sandboxes

- One pod for each task on Kubernetes, so the chart no longer needs a privileged Docker daemon beside the server.
- Docker on a Mac or Windows as a sandbox, through a path to the proxy it can use (getlatentic/planning#666).
- A process sandbox for Windows. macOS (Seatbelt) and Linux (Landlock) have one.
- Tool servers (MCP) inside the virtual machine sandbox; a skill set that names one is refused today.

## More places it runs

- Hermes on Windows (getlatentic/planning#663).
- Podman and rootless Docker.
- The desktop app signed with an Apple and a Windows certificate, and built for Windows and Linux on Arm (getlatentic/planning#601).

## The protocol and the record

- Streams that resume at an exact event, and that are pushed when something changes.
- A turn whose model call failed says it failed (getlatentic/planning#676).
- `gusto verify` inside a sandbox, and the patches of two agents compared side by side.
