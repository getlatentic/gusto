# Installing gusto

Each install runs the same server. `gusto serve` answers the pages, the API and the model proxy from one process, and the pages are compiled into the binary.

| Install | Where | Follow |
| --- | --- | --- |
| `server.sh` | A Linux machine of your own (a VPS, a virtual machine), with HTTPS | [install/server.sh](install/server.sh): `sudo server.sh --domain agents.example.com`, or `--no-tls` on a trusted network. It installs Docker on Debian and Ubuntu, makes a password, starts gusto and says how to sign in |
| Install script | One person's own computer, as a service of theirs | [../docs/LOCAL.md](../docs/LOCAL.md) |
| Ansible | The one box this project runs on, beside other people's sites | [ansible/README.md](ansible/README.md) |
| Compose | One Linux machine of your own | [compose/README.md](compose/README.md) |
| Helm | A Kubernetes cluster of your own | [helm/gusto/README.md](helm/gusto/README.md) |

To run a pilot on somebody else's infrastructure, see [../docs/PILOT.md](../docs/PILOT.md) for what they provide and the checks to run before they sign in.

## What is installed

| Image | Built from | What it is |
| --- | --- | --- |
| server | [server/Dockerfile](server/Dockerfile) | `gusto serve`. Carries `git` and the `docker` CLI, and no agent |
| runner | the root [Dockerfile](../Dockerfile) | The runner and the agents. The server starts one container from it per task and destroys it afterwards |
| runtime | `docker.io/library/docker:29-dind` | Kubernetes only: the Docker daemon beside the server that runs those containers |
| cli | `docker.io/library/docker:29-cli` | Kubernetes only: pulls the runner into that daemon before the server starts |

## Published images

A version tag (`v0.1.0`) runs `.github/workflows/release.yml`. It builds the server and the runner, pushes both to GHCR with an SBOM and build provenance, signs each with the workflow's identity, and lists both digests in the GitHub release. The runner carries the agents and MCP packages that `ansible/group_vars/all.yml` pins.

To install from them, check the signature and set both images in `.env` from the release's notes:

```bash
cosign verify ghcr.io/getlatentic/gusto-server@sha256:<digest> \
  --certificate-identity https://github.com/getlatentic/gusto/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
docker compose pull && docker compose up -d --no-build
```

The chart takes the same digests in `values.yaml`. The images are private, so pulling them needs `docker login ghcr.io` with a token that can read the packages. [helm/gusto/README.md](helm/gusto/README.md#images) has the chart commands.

Base images are pinned by digest. To move one, read the new digest with `docker buildx imagetools inspect docker.io/library/<name>:<tag>` and change the tag and the digest together. The chart refuses a server or runner named by anything but a digest. Compose images are built on the machine and named `gusto-server:local` and `gusto-runner:local`.

## How a task gets its box

`gusto serve` starts every task with `docker run` or `podman run` (`internal/sandbox`): one container per task, destroyed after, sealed with `--network=none` so the proxy's socket is its only way out. Inside Compose or Kubernetes the server is itself in a container, so it needs a container runtime. Each way costs something.

### Compose gives the server the host's runtime socket

The server gets the host's Docker socket, or rootful podman's (`GUSTO_RUNTIME_SOCKET`), and starts each task as a sibling container.

**Whoever holds that socket has root on the host.** Anybody who takes over the server process, through a flaw in gusto or anything it links, has root on the machine. It is shipped because it keeps what gusto promises about a task:

- one container per task, destroyed after, with `--cpus`, `--memory` and `--pids-limit`;
- the seal: `--network=none`, and the proxy's socket mounted in as the only way out;
- no credential and no signing key inside a task.

Put gusto on a machine that runs nothing else you care about. Whoever can change the server's image, its environment or `.env` has the machine. A task does not hold the socket.

Two other options are not shipped:

- Rootless podman inside the server's container needs the host to relax seccomp and AppArmor and to hand over `/dev/fuse`. It trades root-equivalence for a thinner sandbox, and nobody has run gusto that way.
- `GUSTO_SANDBOX=here` runs each task in the server's own process, with no runtime. Calls still go through the proxy at `127.0.0.1:8041` with the task's own token, and the task gets none of the server's password, keys or tokens. A task can read and write whatever the server's user can, other tasks' directories included, and reach any network. In a container the pages cannot use it: the server finds no agent in the runner image, so only `POST /api/jobs` runs a task.

### Kubernetes gives the pod a privileged runtime

A Kubernetes node runs containerd or CRI-O, so there is rarely a Docker socket to mount. The chart puts a Docker daemon (`docker:29-dind`) beside the server in the same pod, and the server starts every task in it.

**The daemon's container is privileged, which is root on the node.** The namespace has to admit it (Pod Security `privileged`), and a flaw in gusto is a way onto that node. The daemon listens on a unix socket in the pod and on no TCP port. The image's default opens 2375 to the pod network with no authentication.

- Tasks run inside the daemon's container, so the scheduler sees one container. `resources.runtime` has to hold `jobs.concurrency` times `jobs.memory`.
- The daemon's storage lives with the pod, so a new pod pulls the runner again.
- `GUSTO_SANDBOX=here` is not offered.

A sandbox that asks the Kubernetes API for a Pod per task would remove the privilege. It needs Go code in `internal/sandbox` and is not part of this install.

## What it needs outbound, and what it does not

[../docs/AIRGAP.md](../docs/AIRGAP.md) names every place gusto reaches outside and what replaces each.

- Building the images reaches what AIRGAP.md's B1 to B5 name, plus `docker.io` for the bases, `proxy.golang.org` for the Go modules, Debian bookworm's mirrors for `git`, `ca-certificates` and (in the server) `openssh-client`, and `registry.npmjs.org` for the agents in `HARNESSES`.
- Installing with Compose reaches nothing more. On Kubernetes the kubelet pulls the server, the runtime and the CLI from the registries the values name, and the runtime pulls the runner whenever a pod starts.
- Running reaches what AIRGAP.md's S1 to S8 and J1 to J7 name: the identity provider if set, the model upstreams the proxy holds credentials for, the tasks' git hosts, and the hosts `GUSTO_ALLOWED_HOSTS` names. An agent calling home from a sealed task is refused and written down. With no model upstream set there is no proxy, and a task keeps the runtime's default network, which on these installs is the Docker bridge and reaches anything the machine does.

gusto calls no license server, sends no telemetry, checks for no updates, and its pages load nothing from outside the server. Only the port the pages are on needs to be reachable. The proxy listens on a unix socket, and the chart's Service exposes the pages alone.

## What has been run, and what has not

Compose was run on Linux x86-64 and linux/arm64: both images build, the server reads `healthy`, `smoke.sh` passes with no upstream, with an upstream set and with a signing key, and a sealed task's request around the proxy is refused. The chart passes `helm lint --strict` and `kubeconform -strict` against Kubernetes 1.30.0, and the `install-check` workflow installs it from a release's digests on a `kind` cluster. No task ran there and no model was reached.

Not run yet:

- the chart on a cluster you run, and its ingress (`install-check` runs with the ingress off);
- Compose on podman's socket, rootful or rootless;
- a real agent with a real model through either install;
- the chart on arm64.
