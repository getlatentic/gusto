# Running an agent on a computer you are given

gusto runs an agent job on any computer that meets this contract. vuxto rents such computers. vux connects to them. This page says what each side provides.

## Roles

| | Does | Does not |
| --- | --- | --- |
| gusto | Packages the agent, holds the model proxy and tool gateway, drives the run, writes the signed record. | Choose or own the computer. |
| vuxto | Rents a computer: lease, credits, the isolated machine, the relay to the proxy, teardown. It stores agent packages by gusto's digest and wires the adapters. | Run an agent's logic or read its prompts. The package lives only on the encrypted disk. |
| vux | Carries bytes between two computers and runs terminals on one. | Decide who may connect. |

## What gusto needs from a computer

Each line is a method of `box.Box` (`pkg/box`).

| # | Need | Method | Provided by |
| --- | --- | --- | --- |
| 1 | Start a program with plain standard input, output and error, and read its exit status. No terminal. The machine image carries the programs the adapter names, installed from gusto's `pins.json`, and the box checks `command -v` before the run. | `Wrap`, `Attach` | vux (`exec`) and the machine image. |
| 2 | Put the workspace and the agent package in, and get the results out. | `Start`, `Equip`, `Fetch` | vux file calls, driven by the box. |
| 3 | Let the agent reach the proxy, and nothing else, with a token. A shim in the machine listens on `127.0.0.1:8080` and `:3128` and carries the bytes over vsock to the lender's relay, which passes them to the proxy over pinned TLS. | `HostModelURL`, `Env` | vuxto. |
| 4 | Keep the machine isolated, and delete it and its disk when the run ends. | `Close` | vuxto. |
| 5 | Report what the machine's own guard saw. | `Guard` | vuxto. It returns the proxy's per-lease call log, fetched through the hub. |

`Workdir` says where the workspace is inside the machine.

## Rules

1. The computer is untrusted. The lender can read everything on it.
2. The model key never enters the machine. The agent holds a lease token that the proxy accepts for that lease only.
3. The record is written and signed outside the machine by `pkg/runner.Outside`, which runs on the renter's computer. Nothing the machine reports is trusted without a check. The lender also sees when the box starts and ends and how large everything crossing its relay is.
4. The token reaches the machine through the renter's own channel (vux file calls to `/run/vuxto/egress-token`), not through the lender's.
5. No state outlives the lease.

## Three places, chosen separately

A run has three places. Each is chosen on its own.

| Place | Is | May be |
| --- | --- | --- |
| Process | The computer the agent program runs on. | Any computer that gives need 1. |
| Working directory | The folder the agent reads and writes. | On the process's own disk, on a network mount from another computer (NFS, sshfs), or copied in and out. |
| Credential | The model key. | Anywhere. It stays with the proxy. The agent holds only the proxy address and a token (need 3). |

These placements all work:

| Process | Working directory | Case |
| --- | --- | --- |
| A | A | The run is local. `runner.Inside` does this today. |
| B | B | Both on a rented computer. The workspace is copied in, and the result is copied out. |
| B | A, mounted on B | The agent runs on a rented computer and edits your folder. |
| A | B, mounted on A | The agent runs here and edits files on another computer. |

The box decides how the working directory is provided. `Start` and `Fetch` copy the folder. A box that mounts the folder makes `Start` mount it, makes `Fetch` do nothing, and returns the mount path from `Workdir`. The agent sees only a path.

## Who imports whom

vuxto imports `gusto/pkg/*`. gusto never imports vuxto. vuxto calls `pkg/runner.OutsideContext` with its own box.

## State

| Part | State |
| --- | --- |
| `pkg/box`, `pkg/runner`, proxy token and credential interfaces | Built on `tosin/public-packages` in the vuxto working copy. Not pushed. |
| Box over a vuxto machine | Not built. |
| Raw run command in vux | In progress on `tosin/exec` in the vux repo: `exec.start/write/read/close` over plain pipes, and `vux -S TARGET exec -- COMMAND`. A pane is a terminal and echoes input, which corrupts the agent's JSON-RPC stream. |
| Machine image with the Claude Code agent program | Not built. The image has the plain `claude` binary, not the node program the adapter runs. |
| Proxy per-lease call log for `Guard` | Not built. Until it is, `Guard` returns an empty events file and a fixed policy. |
| Tool servers inside a box | gusto refuses them for OpenShell sandboxes today. A box that carries them needs a capability check in `internal/runner`. |

## Done means

A Claude Code agent finishes the same task in each placement in the table above. In each, the proxy sees the model calls under a token, the credential is not on the process's computer, and the record verifies with the existing gusto check. A rented machine is gone afterward.
