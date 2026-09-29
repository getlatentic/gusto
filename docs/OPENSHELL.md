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

greencloud is a KVM guest with no `/dev/kvm`, so the VM driver does not run there.
On AWS an `m8i.xlarge` launched with nested virtualization does, and
`deploy/aws/openshell` makes one and removes it.

## Settings

| Setting | Meaning |
| --- | --- |
| `HAAS_SANDBOX=openshell` | run each job's harness in a sandbox |
| `HAAS_OPENSHELL` | the `openshell` command, `openshell` by default |
| `HAAS_OPENSHELL_GATEWAY` | the registered gateway to use, or the selected one |
| `HAAS_OPENSHELL_IMAGE` | what a sandbox is made from: an image reference the gateway can pull, or the path of a root file system archive on the gateway's machine. It carries the harness programs. Unset is the gateway's own image, which carries none |
| `HAAS_OPENSHELL_HOST` | the name a sandbox gives the gateway's machine, `host.openshell.internal` by default |
| `HAAS_OPENSHELL_CPU`, `HAAS_OPENSHELL_MEMORY` | limits for one sandbox, `1` and `3Gi` by default |
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
3. The harness runs in the sandbox through `openshell sandbox exec`. Its output
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
| `/usr`, `/lib`, `/bin`, `/etc`, `/proc` read-only | the system a sandbox needs to start |
| `/sandbox`, `/tmp` and the harness's own folder read-write | the workspace is `/sandbox/workspace` |

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

- **Conversations and tool servers.** A job with a skill set runs; a turn in a
  conversation, or a set with MCP servers, is refused by name before anything
  starts.
- **The rest of the matrix.** opencode and pi have edited a file in a sandbox made
  from the runner image, on a real model, and passed the project's tests. Claude Code,
  codex, dsh and hermes have not run in a sandbox.
- **`haas verify`.** It re-runs the gate where it is run, which for a job that ran
  in a sandbox is the verifier's machine and not a sandbox.
- **A process limit.** The VM driver says it cannot enforce one.

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
