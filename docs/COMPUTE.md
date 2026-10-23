# Running an agent on a computer you are given

gusto runs an agent job on any computer that meets this contract. vuxto rents such computers. vux connects to them. This page says what each side provides.

## Roles

| | Does | Does not |
| --- | --- | --- |
| gusto | Packages the agent, holds the model proxy and tool gateway, drives the run, writes the signed record. | Choose or own the computer. |
| vuxto | Rents a computer: lease, credits, the isolated machine, the relay to the proxy, teardown. | Know what an agent is. |
| vux | Carries bytes between two computers and runs terminals on one. | Decide who may connect. |

## What gusto needs from a computer

Each line is a method of `box.Box` (`pkg/box`).

| # | Need | Method | Provided by |
| --- | --- | --- | --- |
| 1 | Start a program with plain standard input, output and error, and read its exit status. No terminal. | `Wrap`, `Attach` | vux. A raw run command is not built yet. |
| 2 | Put the workspace and the agent package in, and get the results out. | `Start`, `Equip`, `Fetch` | vux file calls, driven by the box. |
| 3 | Let the agent reach the proxy, and nothing else, with a token. | `HostModelURL`, `Env` | vuxto relay inside the machine. |
| 4 | Keep the machine isolated, and delete it and its disk when the run ends. | `Close` | vuxto. |
| 5 | Report what the machine's own guard saw. | `Guard` | vuxto. It returns the proxy's log for the lease. |

`Workdir` says where the workspace is inside the machine.

## Rules

1. The computer is untrusted. The lender can read everything on it.
2. The model key never enters the machine. The agent holds a lease token that the proxy accepts for that lease only.
3. The record is written and signed outside the machine by `pkg/runner.Outside`. Nothing the machine reports is trusted without a check.
4. The token reaches the machine through the renter's own channel, not through the lender's.
5. No state outlives the lease.

## How a run goes

1. vuxto boots the machine and returns a box.
2. The box starts, writes the token into the machine and copies in the workspace.
3. `Equip` places the skills from the agent package. The package digest is the one gusto computes.
4. `Outside` starts the agent through `Attach` and speaks to it over its standard streams.
5. `Fetch` copies the workspace back. `Guard` returns the log. `Outside` signs the record.
6. `Close` deletes the machine.

## Who imports whom

vuxto imports `gusto/pkg/*`. gusto never imports vuxto. vuxto calls `pkg/runner.OutsideContext` with its own box.

## State

| Part | State |
| --- | --- |
| `pkg/box`, `pkg/runner`, proxy token and credential interfaces | Built on `tosin/public-packages` in the vuxto working copy. Not pushed. |
| Box over a vuxto machine | Not built. |
| Raw run command in vux | Not built. A pane is a terminal and echoes input, which corrupts the agent's JSON-RPC stream. |
| Tool servers inside a box | gusto refuses them for OpenShell sandboxes today. A box that carries them needs a capability check in `internal/runner`. |

## Done means

A real Claude Code agent runs a task on a vuxto machine. The proxy sees the model calls under the lease token. The record verifies with the existing gusto check. The machine is gone afterward.
