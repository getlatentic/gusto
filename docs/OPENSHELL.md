# OpenShell sandboxes

gusto can run a task's agent in a sandbox on an [OpenShell](https://github.com/NVIDIA/OpenShell) gateway instead of a container. The sandbox is a microVM with its own kernel. A policy sets what it may reach and write. The signed record holds what the policy allowed and denied. Set `GUSTO_SANDBOX=openshell`. gusto drives the gateway through the `openshell` command and does not modify OpenShell.

## Requirements

- An OpenShell gateway with the VM driver, reachable by `openshell` on the machine that runs `gusto serve`.
- KVM on Linux, or Hypervisor.framework on macOS, on the gateway's machine.
- `allow_driver_config = true` under `[openshell.drivers.vm]`, when the image is a root file system archive.
- A model credential. The sandbox reaches its model only through the gusto proxy.
- An `ssh` client on the machine that runs `gusto serve`. Agents that speak the Agent Client Protocol (dsh, hermes) are reached over ssh through the gateway.

A server with `GUSTO_SANDBOX=openshell` on Linux with no `/dev/kvm` refuses to start, and `gusto doctor` reports it. Set `GUSTO_OPENSHELL_REMOTE=1` when the gateway is on another machine. greencloud has no `/dev/kvm`. Software emulation is too slow and is not supported.

| Where it runs | What makes `/dev/kvm` appear |
| --- | --- |
| A physical server | Turn on VT-x or AMD-V in the BIOS, then `modprobe kvm_intel` or `modprobe kvm_amd`. Add the gateway's user to the `kvm` group |
| A VM on VMware, Proxmox or similar | The hypervisor's administrator exposes hardware virtualization to the VM, then the VM restarts |
| A cloud machine | A type with nested virtualization enabled at launch. On AWS: `c8i`, `m8i` or `r8i` with it on, or a `.metal` type. `deploy/aws/openshell` makes and removes one. Other clouds are untried |
| A container | Pass the device: `--device /dev/kvm` |

## Settings

| Setting | Meaning |
| --- | --- |
| `GUSTO_SANDBOX=openshell` | run each task's agent in a sandbox |
| `GUSTO_OPENSHELL` | the `openshell` command, `openshell` by default |
| `GUSTO_OPENSHELL_REMOTE` | `1` when the gateway is on another machine, so this one needs no `/dev/kvm` |
| `GUSTO_OPENSHELL_GATEWAY` | the registered gateway to use, or the selected one |
| `GUSTO_OPENSHELL_IMAGE` | an image reference the gateway can pull, or the path of a root file system archive on the gateway's machine. It must carry the agent programs. Unset is the gateway's own image, which carries none |
| `GUSTO_OPENSHELL_HOST` | the name a sandbox gives the gateway's machine, `host.openshell.internal` by default |
| `GUSTO_OPENSHELL_CPU`, `GUSTO_OPENSHELL_MEMORY` | limits for one sandbox, `1` and `3Gi` by default |
| `GUSTO_SPARE_COMPUTER` | `on` keeps one blank computer made in the background, so the first task of a person with no computer yet starts without waiting for the gateway to make one (about eight seconds). It holds the memory and CPU of one sandbox while it waits, is made with the same image, limits and policy as a person's, and is removed when the server stops. Off by default |
| `GUSTO_OPENSHELL_PROCESSES` | the most processes an agent's user may have, `512` by default. The VM driver sets no limit, so the start command sets one with `ulimit`. A sandbox where it cannot be set runs nothing |
| `GUSTO_ALLOWED_HOSTS` | hosts a task may reach besides its model, as for containers. Each becomes a network rule |
| `GUSTO_PROXY_ADDR` | the proxy address. In this mode the proxy listens on `127.0.0.1:8041` |

## How a task runs

The runner stays on the host and writes the record itself. It checks out the repository on the host, makes a sandbox with the task's policy, and puts in the workspace and skills. It runs the agent with `openshell sandbox exec`, or over ssh for an ACP agent. The agent environment is the proxy address and the task token only. It fetches the workspace back and takes the patch on the host. The check runs in the sandbox. The runner writes `out/sandbox-events.log` and `out/sandbox-policy.json`, the manifest hashes them, the host signs it, and the sandbox is deleted.

## The policy and the image

The runner writes a policy for each task. Everything not named is denied.

| The policy names | From |
| --- | --- |
| the proxy, `host.openshell.internal` on its port | always. The sandbox holds only a token, never the credential |
| each allowed host and port | `GUSTO_ALLOWED_HOSTS`: `host` is ports 80 and 443, `host:port` is one port, `*.domain` is every name under it |
| `/usr`, `/lib`, `/bin`, `/opt`, `/proc`, read-only | the system a sandbox needs to start |
| part of `/etc`, read-only | users, groups, name resolution, certificates, library cache (`systemReads` in `internal/openshell/policy.go`). A path named there must exist in the image |
| `/sandbox`, `/tmp` and the agent's own folder, read-write | the workspace is `/sandbox/workspace` |
| `/dev/ptmx` and `/dev/pts`, read-write | an agent's shell tool opens a pseudo-terminal per command |

File rules need Landlock. A sandbox that cannot enforce them does not start. A `GUSTO_ALLOWED_HOSTS` entry that cannot be written as a rule is refused.

### A run's credential

Runs are bound unless an admin chooses plain tokens under **Model access in sandboxes** on the admin overview (`plainRunTokens` in the organization's settings). A plain token works from anywhere until the run ends. Settings that cannot be read leave runs bound. The choice is read as each run starts.

An agent in a sandbox holds nothing it could use anywhere else. It is told `http://127.0.0.1:<port>/model/v1`, the address of a forwarder of its run's own, and the word `dpop` where a key would go.

1. The server gives the run a secret (`WayOut.admitBound`). The proxy never accepts the secret as a token.
2. The sandbox runs `gusto boxed --model <proxy> -- agent ...`. It starts `gusto boxed-forward`, which starts `gusto boxed-sign`. The signer makes an Ed25519 key that never leaves its process, and answers the forwarder's requests for proofs over a pipe. The signer's memory is closed to every other process (`PR_SET_DUMPABLE`), the agent among them. The forwarder's is not, because OpenShell reads a process's memory to decide each connection it makes, and refuses one it cannot read. So the signer makes no network calls, and the forwarder holds only the token, which is no use without a proof.
3. The forwarder trades the secret, once, at `POST /model/token`. This is an OAuth token exchange (RFC 8693) with a DPoP proof (RFC 9449). It gets back a run token: a JWT the server signs (RFC 9068 in shape), good for 15 minutes and bound to the key's thumbprint.
4. Every call goes out with `Authorization: DPoP <token>` and a fresh proof, made for that method, address and token. The proxy refuses a proof it has seen, or one more than a minute from now. The forwarder trades the token for a new one before it expires.
5. When the run ends, its token is refused even before it expires. The forwarder ends with the agent: by a pipe the agent holds open, and on Linux by `PR_SET_PDEATHSIG`. The signer ends with the forwarder, which holds its input.

| A run token's claim | What it is |
| --- | --- |
| `sub` | the person the run acts for, who asked |
| `client_id` | the room's agent, or the agent type |
| `run` | the task |
| `authorization_details` | `gusto_run`: the room and `model`, `tools` (RFC 9396) |
| `cnf.jkt` | the thumbprint of the run's key |

Only the agent's command starts a forwarder. The gate and the checks of the image are given no model address, and spend nothing. The code is `internal/runtoken`, `cmd/gusto/boxed.go` and `pkg/proxy/runtokens.go`. A change to these makes a new runner-image key.

### A room's turn in a person's computer

A person's computer is one sandbox that holds all their conversations and their copies of rooms. The sandbox's policy cannot tell those apart, so a room's turn is held further, inside it:

| A room's turn may | Path |
| --- | --- |
| read and write its copy of the room | `/sandbox/jobs/instance-<room>-<agent>` |
| read and write its agent's home, which keeps what the agent remembers between turns | `/sandbox/rooms/<room>-<agent>/home`, its `HOME` |
| read and write its own temporary folder | `/sandbox/rooms/<room>-<agent>/tmp`, its `TMPDIR` |
| read the system folders | `landlock.System` |

It cannot read or write the rest of `/sandbox`, the person's other conversations and rooms among it, nor the shared `/tmp`.

- The runner starts the agent as `gusto boxed --model <proxy> --policy '<policy>' -- agent ...`, and the turn's other commands as `gusto sandbox-run '<policy>' -- ...`. The gusto in the image applies the policy with Landlock, on top of the sandbox's own, before it starts the command.
- The policy is passed whole, not as a file, so the agent cannot change its next turn's rules.
- A kernel without Landlock refuses the turn rather than running it unconfined.
- `sandbox-run` is part of the image, so a change to it, or to `internal/landlock`, makes a new runner-image key (`deploy/aws/openshell/image-key.sh`).

Export the container runner's image as a root file system (`podman export`) and set `GUSTO_OPENSHELL_IMAGE` to it. The backend copies each agent's configuration from `/opt/gusto/harness` into `/sandbox`, and rewrites `/root` to `/sandbox` in the agent's arguments and configuration files. It creates the agent's state folder before the agent starts. It applies an adapter's `whenBoxed` changes: codex gets `danger-full-access` instead of `workspace-write`, because its own namespace sandbox fails inside a sandbox. The gateway's policy is the boundary. The image carries `ripgrep`, because opencode's `glob` and `grep` cannot download it without a network.

## Measured

OpenShell 0.1.2, on an `m8i.xlarge` with nested virtualization. Scripts are in `deploy/aws/openshell`.

- A first sandbox (512 MB image) ran a command in 8.2 s. A warm one took 6.5 s.
- Thirteen agent and model pairs finished the sample project's one-line fix, in 27 s to 48 s each. The agents were opencode, codex, claude, pi, dsh and hermes. The models were Qwen3 VL 235B, DeepSeek Flash and DeepSeek V4 Pro. The plan is `docs/matrix/box.json`.
- Only pi, dsh and hermes reported tokens the proxy could count. No price is set, so no cost is in dollars.
- opencode on Qwen3 VL passed one of two runs before the image carried `ripgrep`, and four of four after. The cause of the failed run is unknown.
- Claude Code then Codex worked on one project in one sandbox, and its tests passed (3 of 3). Each turn had its own token, so the proxy told their calls apart. The sandbox denied hosts the agents tried on their own, such as `models.opencode.ai`, `registry.npmjs.org` and `github.com`.
- An upload leaves `.git` behind. Take a turn's diff on the host after fetching the workspace.

### Many sandboxes at once

On an `m8i.2xlarge` (8 vCPU, 32 GiB), with 2 CPUs and 3 GiB for each sandbox and the 1.6 GB image. The work was a fixed CPU loop, not an agent.

| Running | Ready in | The same work in | Host memory in use |
| --- | --- | --- | --- |
| 1 | 19 s | 2.5 s | 1.3 GB |
| 2 | 18 to 20 s | 2.5 s | 1.6 GB |
| 4 | 20 to 22 s | 2.5 to 2.6 s | 2.2 GB |
| 8 | 20 to 24 s | 5.1 to 5.4 s | 3.3 GB |
| 12 | 23 to 34 s | 7.3 to 8.2 s | 4.4 GB |

Speed holds to four sandboxes and slows in proportion beyond that. The gateway stages about four root file system archives at once. A fifth made at the same moment is refused with `failed to allocate a rootfs tar staging slot on the gateway`. gusto asks again for up to two minutes. Creating sandboxes in waves of four reached 12 running. Not measured: agents at these counts, a `.metal` instance, and memory under a real workload.

## Share an app from a sandbox

On an agent's page, "Share an app this agent built" takes the start command and the port. The server runs the command in the agent's folder with `PORT` and `HOST=0.0.0.0` set, gives the port an address on the gateway, and shows the link once something answers. Only signed-in members of the organization can open it. A stopped or unresponsive app no longer opens.

- The gateway's own address has no sign-in and is never given to a person. The server asks it as itself, without the person's credentials.
- Behind gusto cloud, each app has its own host name, `<organization>--<id>.<domain>`. A cookie the app sets stays on that host name. One named like the organization's session cookie, or with a `__Host-` prefix, is dropped.
- With no gusto cloud, an app is the path `/apps/<id>/`. It runs under `Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups allow-modals`, and its links must be relative.
- In the sandbox, a server on `::` (Node's `listen(port)`) resets an IPv4 client at `127.0.0.1`. Listen on `0.0.0.0` or `127.0.0.1`. `localhost` does not resolve.

## Run it on a Mac

gusto can fetch, check, sign and start OpenShell in user space, with no password, Homebrew or permission prompt. The person agrees once to a download of about 72 MB plus a 300 MB to 1 GB root file system. Apple Silicon only. Measured on an M1 Max with OpenShell v0.1.3:

- Download, 6 s. Sign the driver with `codesign -s -`, 0.3 s. The gateway listens about 1 s after start.
- The first sandbox took 27.7 s, and one with the image staged took 9.6 s. A chat took 13 s, and each turn gets a fresh VM.
- The gateway kept 2.2 GB after a few runs, and 6.0 GB after one chat with the six-agent image.

`gusto local sandbox install` writes a launch agent (macOS) or systemd user unit (Linux) named `gusto`. `gusto local sandbox uninstall` removes it. The driver needs `mke2fs`, `debugfs` and `e2fsck`. `deploy/e2fsprogs/build.sh` builds them static from e2fsprogs 1.47.4 (GPLv2). The release workflow builds `gusto-rootfs-<arch>.tar.gz` for amd64 and arm64. Neither release task has run. Not checked: a Mac with no e2fsprogs at all, Linux with KVM, and a gateway kept for days.
