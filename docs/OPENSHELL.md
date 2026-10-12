# Running agents in OpenShell sandboxes

haas can run a job's harness in a sandbox on an [OpenShell](https://github.com/NVIDIA/OpenShell)
gateway instead of in a container. The sandbox is a microVM with its own kernel.
What it may reach and write is a policy, and what the policy allowed and denied
is written into the signed record.

Set `HAAS_SANDBOX=openshell` and haas drives the gateway through the `openshell`
command. It uses OpenShell as a component below it and does not modify it.

## What it needs

| Need | Why |
| --- | --- |
| An OpenShell gateway with the VM driver, reachable by the `openshell` command on the machine that runs `haas serve` | The gateway makes the sandboxes |
| Hardware virtualization on the gateway's machine: KVM on Linux, Hypervisor.framework on macOS | The VM driver needs it |
| `allow_driver_config = true` under `[openshell.drivers.vm]`, when the image is a root file system archive | The gateway takes an archive from a caller only when its administrator has said so |
| A model credential | The sandbox reaches its model only through haas's proxy |
| An `ssh` client on the machine that runs `haas serve` | An agent that speaks the Agent Client Protocol (dsh, hermes) is spoken to over ssh through the gateway. `openshell sandbox exec` carries standard input as one piece when it ends, so the agent would never hear its first message |

greencloud is a KVM guest with no `/dev/kvm`, so the VM driver does not run there.
A server started with `HAAS_SANDBOX=openshell` on a Linux machine with no `/dev/kvm` refuses to start and says so, and `haas doctor` reports it. When the gateway runs on another machine, set `HAAS_OPENSHELL_REMOTE=1`.

`/dev/kvm` appears when the kernel's KVM module loads, and that works only if the CPU's virtualization features reach the operating system. Haas cannot make it. What does depends on where the machine runs:

| Where it runs | What makes `/dev/kvm` appear |
| --- | --- |
| A physical server | Turn on VT-x or AMD-V in the BIOS, then `modprobe kvm_intel` or `modprobe kvm_amd`. The user that runs the gateway needs to be in the `kvm` group |
| A virtual machine on VMware, Proxmox or similar | The hypervisor's administrator exposes hardware virtualization to the VM, and the VM restarts |
| A cloud machine | A type that supports nested virtualization, with it enabled at launch. On AWS that is `c8i`, `m8i` or `r8i` with nested virtualization on, or a `.metal` type (`deploy/aws/openshell` does this). Other clouds have their own list, which this repository has not tried |
| A container | The host's device passed in, as `--device /dev/kvm` |

Emulating the hardware in software is too slow to run an agent on, and is not supported.
On AWS an `m8i.xlarge` launched with nested virtualization does, and
`deploy/aws/openshell` makes one and removes it.

## Settings

| Setting | Meaning |
| --- | --- |
| `HAAS_SANDBOX=openshell` | run each job's harness in a sandbox |
| `HAAS_OPENSHELL` | the `openshell` command, `openshell` by default |
| `HAAS_OPENSHELL_REMOTE` | `1` when the gateway runs on another machine, so this one needs no `/dev/kvm` |
| `HAAS_OPENSHELL_GATEWAY` | the registered gateway to use, or the selected one |
| `HAAS_OPENSHELL_IMAGE` | what a sandbox is made from: an image reference the gateway can pull, or the path of a root file system archive on the gateway's machine. It carries the harness programs. Unset is the gateway's own image, which carries none |
| `HAAS_OPENSHELL_HOST` | the name a sandbox gives the gateway's machine, `host.openshell.internal` by default |
| `HAAS_OPENSHELL_CPU`, `HAAS_OPENSHELL_MEMORY` | limits for one sandbox, `1` and `3Gi` by default |
| `HAAS_OPENSHELL_PROCESSES` | the most processes a harness's user may have in a sandbox, `512` by default. The VM driver enforces no limit of its own, so the command that starts a harness sets it with `ulimit`, and a sandbox where it cannot be set runs nothing |
| `HAAS_ALLOWED_HOSTS` | the hosts a job may reach besides its model, as for containers; each becomes a network rule |

The proxy listens on `127.0.0.1:8041` in this mode, and `HAAS_PROXY_ADDR` changes
it. The sandbox reaches the machine's loopback through the gateway's own proxy,
so the proxy need not be on the network.

## How a job runs

The runner stays on the machine that runs the server, outside the sandbox, and
writes the record itself:

1. The repository is checked out on the host, as for a container.
2. A sandbox is made with the job's policy, and the workspace and the job's
   skills are put in it.
3. The harness runs in the sandbox through `openshell sandbox exec`, or over ssh
   through the gateway when it is an ACP agent. Its output
   is the record's transcript, written on the host. Its environment is the proxy
   address and the per-job token and nothing of the server's own.
4. The workspace comes back, links kept as links and not followed, and the patch
   is taken from it on the host.
5. The gate runs in the sandbox, where the repository's own code is.
6. The sandbox's log and the policy it enforced are written to
   `out/sandbox-events.log` and `out/sandbox-policy.json`, the manifest hashes
   them with the rest of `out/`, and the host signs the manifest.
7. The sandbox is deleted.

A harness in the sandbox has no record to write: it is written on the host, after
the harness has finished, by a process the sandbox cannot reach.

## The policy

A policy is written for each job. Everything not named is denied.

| A policy names | From |
| --- | --- |
| the proxy, `host.openshell.internal` on its port | always. It holds the model credential, and the sandbox has only a token that stands in for it |
| each allowed host and port | `HAAS_ALLOWED_HOSTS`: a host is ports 80 and 443, `host:port` is one, and `*.domain` is every name under it |
| `/usr`, `/lib`, `/bin`, `/opt`, `/proc` read-only | the system a sandbox needs to start |
| of `/etc`, only what a program needs, read-only | users and groups, name resolution, certificates and the library cache (`systemReads` in `internal/openshell/policy.go`). `/etc/shadow`, the rest of `/etc` and `/var/log` are not readable. A path named here must exist in the image, or the sandbox does not start |
| `/sandbox`, `/tmp` and the harness's own folder read-write | the workspace is `/sandbox/workspace` |
| `/dev/ptmx` and `/dev/pts` read-write | a harness's shell tool opens a pseudo-terminal for each command. codex failed every one without them |

The file rules need Landlock, and a sandbox that cannot enforce them does not
start. An entry of `HAAS_ALLOWED_HOSTS` that cannot be written as a rule is
refused and not dropped.

## The runner image as a sandbox

The image the container runner uses is exported as a root file system and
serves as the sandbox's image (`podman export`, then `HAAS_OPENSHELL_IMAGE`). For a
harness written for a container, three things are done:

- The image keeps a second copy of each harness's configuration under
  `/opt/haas/harness`, where the sandbox user can read it, and the backend
  copies it into the user's home, `/sandbox`, before the harness starts.
- Every mention of `/root` in what a harness is told, the paths of its state and
  its skills among them, is rewritten to `/sandbox`.
- The program is asked of the sandbox and not of the machine that runs the server.
- The copies of a harness's configuration are rewritten the same way. hermes names
  `/root/.agents/skills` in a configuration file, where a rewrite of the command
  line does not reach.
- The harness's own state folder is made before it starts, as in a container.
  codex refuses a home that does not exist.
- An adapter can say what to change when the box confines the harness already
  (`whenBoxed`). codex builds its own sandbox from namespaces, which a sandbox that
  forbids them cannot allow, and every command it ran failed with `bwrap: No
  permissions to create a new namespace`. Inside a gateway's sandbox it is given
  `danger-full-access` in place of `workspace-write`, and the gateway's policy is
  the boundary.
- The image carries `ripgrep`. opencode's `glob` and `grep` tools download one on
  first use, which a box with no network cannot, so every search failed and the
  model fell back to `find`.

A harness reaches the network only through the policy, so what a harness does on
its own is denied and recorded. opencode asks `models.opencode.ai` and
`registry.npmjs.org` on every start, and both were denied without harm.

## What is in the record

`out/sandbox-events.log` is the gateway's and the sandbox's log for the run. On the
first test it held, for the harness that asked the model for an answer and then
tried a host nobody allowed:

```
NET:OPEN  ALLOWED /usr/bin/bash -> host.openshell.internal:8041 [policy:haas_model_proxy]
HTTP:POST ALLOWED POST http://host.openshell.internal:8041/model/v1/chat/completions
NET:OPEN  DENIED  /usr/bin/bash -> example.com:443 [reason:transparent_tcp_policy_denied]
```

The log is read again until it stops growing, because a sandbox ships it in
batches and the last moment of a run arrives late.

## What it does not do yet

- **Tool servers.** A job with a skill set runs; a set with MCP servers is refused by
  name before anything starts.
- **`haas verify`.** It re-runs the gate where it is run, which for a job that ran
  in a sandbox is the verifier's machine and not a sandbox.

## Measured

On an `m8i.xlarge` with nested virtualization, OpenShell 0.1.2: a first sandbox,
pulling a 512 MB image, started and ran a command in 8.2 s and a warm one in
6.5 s. A comparison of two engines on the sample project took 9 s a run. The
scripts are in `deploy/aws/openshell`: `probe.sh`, `e2e.sh` and `e2e-compare.sh`.

On a real model, with the key read from Parameter Store by the instance
(`e2e-real.sh`): Qwen3 VL 235B on Bedrock, through the proxy. opencode fixed the
sample project's discount and the tests passed. A comparison of opencode and pi on
it passed both, in 39 s and 18 s, each with a one-line diff and a signed record.
opencode asked for `models.opencode.ai`, `registry.npmjs.org` and `github.com`;
each was denied and it carried on. The credential name has to be one the harness's
adapter knows: opencode failed under `bedrock`, which its adapter has no provider
for, and passed under `platform`.

The plan in `docs/matrix/box.json`, run with `e2e-matrix.sh` from one image that carries
all six harnesses: thirteen harness and model pairs on the sample project's one-line
fix, each in its own sandbox through the proxy, on Qwen3 VL 235B (Bedrock) and on
DeepSeek Flash and V4 Pro. All thirteen finished, in 27 s to 48 s each.

| harness | models | wall clock |
| --- | --- | --- |
| opencode | Qwen3 VL, DeepSeek Flash, DeepSeek V4 Pro | 34 s to 48 s |
| codex | DeepSeek Flash, DeepSeek V4 Pro | 28 s, 44 s |
| claude | DeepSeek Flash, DeepSeek V4 Pro, through the Anthropic-shaped address | 29 s, 38 s |
| pi | DeepSeek Flash, DeepSeek V4 Pro | 27 s, 34 s |
| dsh | DeepSeek Flash, DeepSeek V4 Pro | 40 s, 40 s |
| hermes | DeepSeek Flash, DeepSeek V4 Pro | 40 s, 43 s |

Billed tokens are the proxy's count, and only pi, dsh and hermes left one: the other
harnesses' calls read "unmeasured", and no price is set for any of these models, so
no cost is in dollars. Before the image carried `ripgrep`, opencode on Qwen3 VL passed
in one of two runs; once `glob` and `grep` worked it passed in four of four. In the
failed run `find . -name invoice.js` printed nothing in a workspace that held the file,
and it did not happen again. The cause is not known.

## One sandbox for several harnesses, a shared app, and many sandboxes

Measured on 2026-09-29 on an `m8i.2xlarge` (8 vCPU, 32 GiB) with the runner image that
carries all six harnesses. The scripts are `e2e-shared.sh` and `e2e-concurrency.sh` in
`deploy/aws/openshell`.

**Two harnesses, one sandbox.** Claude Code and then Codex worked on one project in one
sandbox. Claude wrote `server.js`, a page that shows an invoice total. Codex fixed the
discount in `invoice.js` and added a `/health` route to `server.js`. The project's
tests then passed, 3 of 3. The turns took 45 s and 31 s. Each turn had a token of its
own, and the proxy recorded 16 calls through the `anthropic` credential for the first
and 13 through `deepseek` for the second, so the calls of a turn can be told apart. Between
them the harnesses asked for `api.anthropic.com`, `chatgpt.com`, `ab.chatgpt.com`,
`github.com` and `api.github.com`, and the sandbox denied each.

An upload leaves `.git` behind, so a sandbox holds the files and no history. The diff of
a turn is taken on the host after the workspace is fetched, or from a repository made in
the sandbox before the turn. Both harnesses ran without one.

**A web app shared from a sandbox.** `openshell service expose <sandbox> <port> <name>`
gives the service a URL on the gateway, `http://default--<sandbox>--<name>.openshell.localhost:18080/`,
and the page the harnesses wrote answered there with HTTP 200. Through an SSH tunnel to the
gateway's port it was drawn in a browser. `openshell forward start` gave the same page on
a port of the host. Asked with no credential and only the host name, the gateway answered,
so the URL has no sign-in of its own, and sharing with colleagues has to go through the
organisation's own pages.

haas shares such an app for a person. On an agent's page, "Share an app this agent built" takes the line that
starts it and the port it listens on. The organisation's server runs the command in the agent's
folder in the person's computer, with `PORT` and `HOST=0.0.0.0` set, gives the port an address on the
gateway, and asks it until something answers. Only then is the link shown, and only signed-in
members of the organisation can open it. An app that stops answering, and one that is stopped,
no longer opens. A computer with an app people use is not stopped for being idle. Every agent is
also given a skill, `haas-sandbox`, that says what follows.

The gateway's own address has no sign-in and is never given to a person: the server asks it as
itself, with the person's credentials taken out of the request. The gateway also refuses a request
whose `Origin` is not its own, which stops every form or `fetch` a browser posts, so the server passes
a request on as from the gateway's own origin.

Behind a front door each app has a host name of its own, `<organisation>--<id>.<domain>`, one label
under the wildcard certificate, so the page an agent wrote has an origin of its own and its absolute
paths, such as `fetch('/click')`, are its own. The first app an agent built used one, and did not work
at a path of the organisation's address. The front door signs a header that says which app a request
is for, and a request with it reaches nothing but that app. A person is signed in on the app's name with
the same token that signs them in to the organisation, and a stranger is sent to the front door. What
the app sets is its own: a cookie is kept for that host name alone, and one with the organisation's
session name or a `__Host-` prefix is dropped. The organisation's session cookie is named `__Host-haas`
over TLS, which a sibling host name cannot set. Checked live on 2026-09-30: an agent built a counter, it
was shared, and its button counted through the app's own name.

On a server with no front door an app is a path of the organisation's address, `/apps/<id>/`, and
what comes back is sent with `Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups
allow-modals`, so it runs with no origin and cannot read or act on the person's session. Its links have
to be relative there.

What the app has to know about the sandbox's network:

- A server that listens on `::`, which is what Node's `listen(port)` does, resets an IPv4
  client at `127.0.0.1` and answers one at `::1`. One that listens on `0.0.0.0` or
  `127.0.0.1` answers on `127.0.0.1`.
- `localhost` does not resolve.
- A process started with `setsid nohup` in one `exec` was still there in the next.

The first app failed for the first reason, and Claude said so in its own answer.

**Several sandboxes at once.** A gateway makes many microVMs on its host. These sandboxes
had 2 CPUs and 3 GiB each. Every one took 18 to 24 s to be ready at 1, 2, 4 and 8 running,
and up to 34 s at 12, with the 1.6 GB image. A fixed piece of CPU work took 2.5 s in one
sandbox and 2.5 to 2.6 s with four running. It took 5.1 to 5.4 s with eight and
7.3 to 8.2 s with twelve: about what four cores' worth of CPU shared among them gives, and
the host has 8 vCPUs, which are threads and not eight cores. That reading is an inference
from the times, and the core count was not checked.

| Running | Ready in | The same work in | Host memory in use |
| --- | --- | --- | --- |
| 1 | 19 s | 2.5 s | 1.3 GB |
| 2 | 18 to 20 s | 2.5 s | 1.6 GB |
| 4 | 20 to 22 s | 2.5 to 2.6 s | 2.2 GB |
| 8 | 20 to 24 s | 5.1 to 5.4 s | 3.3 GB |
| 12 | 23 to 34 s | 7.3 to 8.2 s | 4.4 GB |

The gateway stages about four root file system archives at once. A fifth sandbox made
from an archive at the same moment was refused with `failed to allocate a rootfs tar
staging slot on the gateway`, at 6 and at 8 created together. Sandboxes that were
running were not affected, and the request is accepted a moment later, so haas asks again
for up to two minutes. Making sandboxes in waves of four reached 12 running.

Not measured: harnesses at these counts (the work was one busy loop and not an agent, and
agents wait for a model most of the time), a `.metal` instance, and memory under a real
workload.

## Shipping it with haas on a Mac (spike, 12 October 2026)

**Question.** Can haas bring OpenShell's gateway and VM driver with it, so that a Mac gets virtual machine sandboxes with no setup?

**Decision.** Yes for everything but one tool. haas can fetch, check, sign and start OpenShell itself, in user space, with no password, no Homebrew and no permission prompt. What it cannot do alone is supply e2fsprogs, and it needs a root file system for the agents. Both are work for haas to do, not for the person; the person's part is one consent to a download.

Measured on an M1 Max, macOS 27.2, OpenShell v0.1.3, in a scratch folder with nothing installed outside it. This Mac already had e2fsprogs from Homebrew, so it is not a clean Mac, and a Mac cannot be tested clean in a virtual machine: Hypervisor.framework does not run in a macOS guest on an M1.

| What | Size or time |
| --- | --- |
| Three release files, `openshell`, `openshell-gateway`, `openshell-driver-vm` (Apple Silicon only: there is no Intel Mac build) | 72 MB to download, 6 seconds; 137 MB on disk; each checked against the release's checksum file |
| Sign the driver with the hypervisor entitlement (ad-hoc, `codesign -s -`) | 0.3 seconds; no certificate, no keychain |
| Make the certificates (`openshell-gateway generate-certs`) | 0.05 seconds |
| Write the gateway's configuration, start it, register the CLI | the gateway is listening about 1 second after it starts |
| Idle | gateway 42 MB, driver 22 MB of memory |
| Root file system: the runner image with opencode, exported | 853 MB as a tar, 310 MB as `.tar.gz`, which the gateway accepts |
| First sandbox, cold (staging the 310 MB archive, building its disk) | 27.7 seconds |
| A sandbox once the image is staged | 9.6 seconds |
| A chat answered through a sandbox, from haas started with no `HAAS_SANDBOX` (it chose the VM itself) | 13 seconds; each turn gets a fresh VM |
| Disk the gateway keeps after a few runs | 2.2 GB, mostly the cached image and sandbox disks |

**The steps, from nothing.** These are the formula's own, and nothing in them needs the person:

1. Download the three files and verify them. They arrive without Apple's quarantine mark when a program downloads them, and they are only ad-hoc signed and not notarized, so a browser download would be stopped by Gatekeeper and haas's own download is not.
2. Sign a copy of the driver with `com.apple.security.hypervisor` and `codesign --force -s -`.
3. `openshell-gateway generate-certs --output-dir <state>/tls --server-san host.openshell.internal`.
4. Write the gateway configuration (the keys that v0.1.3 needs, each found by the gateway refusing without it):

```toml
[openshell]
version = 2

[openshell.gateway]
compute_driver = "vm"
guest_tls_ca = "<state>/tls/ca.crt"

[openshell.drivers.vm]
allow_driver_config = true
```

5. Start `openshell-gateway` with `OPENSHELL_GATEWAY_CONFIG`, `OPENSHELL_TLS_CERT`, `OPENSHELL_TLS_KEY`, `OPENSHELL_TLS_CLIENT_CA` and `OPENSHELL_LOCAL_TLS_DIR` (where the JWT signing keys are; without it a sandbox will not launch), and `XDG_STATE_HOME` short, since the driver's socket path must be under 104 bytes. The gateway finds `openshell-driver-vm` in the `libexec` folder beside its own `bin`.
6. Put the client certificates where the CLI looks (`<config>/openshell/gateways/openshell/mtls/{ca.crt,tls.crt,tls.key}`, from the same `generate-certs`) and run `openshell gateway add https://localhost:17670 --local --name openshell`.
7. Set `HAAS_OPENSHELL_IMAGE` to the root file system archive. haas then chooses the VM wall by itself: `internal/isolation` found the hardware, the command, the image setting and a gateway that answers `openshell sandbox list`, which tested that probe on a real gateway for the first time.

**What the person still has to do, or what haas has to do for them**

| Step | Why | Plan |
| --- | --- | --- |
| Agree to a download of about 72 MB plus the root file system, 300 MB to 1 GB | haas does not pull a gigabyte unasked | One consent, once |
| e2fsprogs: the driver runs `mke2fs`, `debugfs` and `e2fsck` on the host to build each sandbox's disk, and the gateway fails its preflight without them | They are not on a Mac. Homebrew's build links Homebrew's `libext2fs`, `libcom_err` and gettext, so the three binaries cannot be copied alone | haas builds them once, static, in CI and ships them beside the gateway, as GPLv2 binaries with their source offered. Until then: `brew install e2fsprogs`. Not yet built or tested |
| A root file system for an arm64 Linux guest | The release workflow builds the runner image for `linux/amd64` only, and an arm64 Mac needs `linux/arm64` | Build the arm64 image in the release, with all six engines, and publish it as a `.tar.gz` with its checksum. About 300 MB with one engine; the six-engine size is not measured |
| Start the gateway at login | It is a service | A launch agent, as for haas itself. Not built |

**Found while running it, and fixed.** A new conversation's workspace holds nothing but its git directory. `openshell sandbox upload` refuses an upload that selects no file, so every first turn of a local chat failed. A workspace with nothing in it is now made in the sandbox with `mkdir` and not uploaded (`holdsNothing`), with a test.

**Not checked.** A Mac with no Homebrew and no e2fsprogs at all (needs the static build, and a clean Mac). Linux with KVM, where there is no signing step and the same files are `openshell-gateway-aarch64-unknown-linux-gnu` and its driver. Six engines in the root file system. A gateway kept for days: restarts, and the 2.2 GB of cache. What a sandbox's policy refused, which the record keeps in `sandbox-events.log`.

**Licence.** OpenShell is Apache-2.0. e2fsprogs' `mke2fs`, `debugfs` and `e2fsck` are GPLv2 (the libraries LGPLv2): shipped as separate programs with a source offer, which is the ordinary way.
