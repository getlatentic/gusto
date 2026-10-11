# Installing haas

One product, four ways to install it. Each installs the same server, and the
server is the whole product: `haas serve` answers the pages, the API and the
model proxy from one process. The pages' templates and assets are compiled into
the binary (`internal/web/static/PROVENANCE.md` says so), so there is no second
origin and no front end to build.

| install | where | follow |
| --- | --- | --- |
| Install script | one person's own computer, as a service of theirs | [../docs/LOCAL.md](../docs/LOCAL.md) |
| Ansible | the one box this project runs on, beside other people's sites | [ansible/README.md](ansible/README.md) |
| Compose | one Linux machine of your own | [compose/README.md](compose/README.md) |
| Helm | a Kubernetes cluster of your own | [helm/haas/README.md](helm/haas/README.md) |

To run a pilot for somebody else on their infrastructure, [../docs/PILOT.md](../docs/PILOT.md) lists what they provide and the checks to run before they sign in.

## What is installed

| image | built from | what it is |
| --- | --- | --- |
| server | [server/Dockerfile](server/Dockerfile) | `haas serve`: the pages, the API and the proxy. It carries `git`, because a job's repository is cloned before its box starts, and the `docker` CLI, which starts the box. It carries no harness |
| runner | the root [Dockerfile](../Dockerfile) | the runner and the harnesses. The server starts one container from it per job and destroys it afterwards |
| runtime | `docker.io/library/docker:29-dind` | Kubernetes only: the Docker daemon beside the server that runs those containers |
| cli | `docker.io/library/docker:29-cli` | Kubernetes only: pulls the runner into that daemon before the server starts |

Both installs build the server and the runner from this repository, so the two
carry the same `haas`.

## Published images

A version tag (`v0.1.0`) runs `.github/workflows/release.yml`. It builds the
server and the runner, pushes both to GHCR with an SBOM and build provenance,
signs each with the workflow's own identity, and lists both digests in the
GitHub release. The runner carries the harnesses and MCP packages that
`ansible/group_vars/all.yml` pins.

To install from them instead of building, check the signature and set both
images in `.env`, from the release's notes:

```bash
cosign verify ghcr.io/getlatentic/haas-server@sha256:<digest> \
  --certificate-identity https://github.com/getlatentic/haas/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
docker compose pull && docker compose up -d --no-build
```

The chart takes the same digests in `values.yaml`. The repository is private,
so its images are too: pulling them needs `docker login ghcr.io` with a token
that can read the packages.

## Every image by digest

Every base image is named by digest:

- `server/Dockerfile` pins `golang:1.26`, `docker:29-cli` and
  `debian:bookworm-slim` on its `FROM` lines.
- The root `Dockerfile` names `golang:1.26` and `node:24-slim` by tag. Compose
  builds it with a named build context for each, which replaces what the `FROM`
  line resolves to with the same image by digest, so the file is unchanged and
  the bases are pinned (`compose/compose.yaml`, `additional_contexts`).
- The chart pins `docker:29-dind` and `docker:29-cli` in `values.yaml`, and it
  refuses to render a server or a runner named by anything but a digest.

What compose builds for itself is named by the tag compose gives it
(`haas-server:local`, `haas-runner:local`): an image built on the machine that
runs it, from pinned bases, never pulled. What Kubernetes runs is pulled, so the
chart takes the digest the registry answered when the image was pushed.
[helm/haas/README.md](helm/haas/README.md#images) has the commands.

To move a base, read the new digest with
`docker buildx imagetools inspect docker.io/library/<name>:<tag>` and change
the tag and the digest together.

## How a job gets its box

On the box, `haas serve` starts every job with `docker run` or `podman run`
(`internal/sandbox`): one container per job, destroyed after, sealed with
`--network=none` so the proxy's socket is its only way out. Inside compose or
Kubernetes the server is itself in a container, so something has to hand it a
container runtime, and every way of doing that costs something. This is what
each one costs, and which one each install ships.

### Compose ships the host's runtime socket

The server's container is given the host's Docker socket, or rootful podman's
(`HAAS_RUNTIME_SOCKET`), and starts each job as a sibling container on the host.

**Whoever holds that socket holds root on the host.** It can start a privileged
container that mounts `/`, and the server holds it. Anybody who takes over the
server process, through a flaw in haas or in anything it links, has root on the
machine. It is not safe. It is shipped because it is the one option that keeps
everything the product promises about a job:

- one container per job, destroyed after, with `--cpus`, `--memory` and
  `--pids-limit`;
- the seal: `--network=none`, and the proxy's socket mounted in as the only way
  out;
- no credential and no signing key inside a job.

So put haas on a machine that runs nothing else you care about, and treat the
server's container as root on it: whoever can change its image, its
environment or `.env` has the machine. A job does not hold the socket, so
signing in and running jobs is not the same as holding it.

The other two options, and why neither is shipped:

- **Rootless podman inside the server's container.** Nested podman needs the
  host to relax seccomp and AppArmor for the server's container and to hand it
  `/dev/fuse`. Podman's own guide runs it without `--privileged`, but what it
  relaxes are the defences the kernel keeps between a container and its host,
  so it trades root-equivalence for a thinner wall and more moving parts, and
  nobody has run haas that way.
- **`HAAS_SANDBOX=here`**, which runs each job in the server's own process. It
  needs no runtime at all, and it gives up everything above: a job runs with the
  server's environment (the shared password, the model key, the signing key), it
  can read every other job's directory, and it reaches the model directly, so
  the proxy never sees it: no `out/proxy.jsonl`, no caps, no allowlist. And
  today the pages cannot use it in a container: the server asks the runtime
  what the runner image carries when it starts, finds none, and offers no
  harness. A run on 2026-09-27 printed `not offered: the runner image could not
  be asked what it carries` for every one. Only `POST /api/jobs` would run a
  job.

### Kubernetes ships a privileged runtime in the pod

A Kubernetes node runs containerd or CRI-O, not Docker, so there is rarely a
Docker socket on the node to mount, and a hostPath to the node's runtime socket
would be root on the node with jobs hidden from the kubelet. The chart instead
puts a Docker daemon beside the server in the same pod (`docker:29-dind`), and
the server starts every job in it.

**The daemon's container is privileged, which is root on the node that runs
it.** The namespace has to admit it (Pod Security `privileged`), and a flaw in
haas is a way onto that node. The image's documentation runs its rootless
variant privileged as well, so that variant does not change this. What it keeps is what the
socket keeps in compose: a container per job, the seal, and no credential in a
job. What it adds is that the daemon listens on a unix socket in the pod and on
no TCP port, because the image's default opens 2375 to the pod network with no
authentication.

What else it costs:

- Jobs run inside the daemon's container, so the scheduler sees one container.
  `resources.runtime` has to hold `jobs.concurrency` times `jobs.memory`.
- The daemon's storage lives with the pod, so a new pod pulls the runner again.

`HAAS_SANDBOX=here` gives up what it gives up in compose, for the same reasons,
and is not offered.

### What a real answer on Kubernetes needs

A sandbox that asks the Kubernetes API for a Pod per job, from the runner image,
instead of asking a daemon for a container. The scheduler then places and
accounts every job, nothing is privileged, and a NetworkPolicy does what
`--network=none` does. That is Go code in `internal/sandbox`: the job directory
has to reach a pod on another node, the proxy has to be reachable from that pod
and from nothing else, the server needs a service account that may create pods
in one namespace, and the catalog has to ask a pod what the image carries. It
is a candidate issue, not part of this install.

## What it needs outbound, and what it does not

[docs/AIRGAP.md](../docs/AIRGAP.md) names every place haas reaches outside and
what replaces each one. For these two installs:

**Building the images** reaches everything AIRGAP.md's B1 to B5 name, plus the
server's own: `docker.io` for the bases pinned above, `proxy.golang.org` for the
Go modules (both Dockerfiles download them), Debian bookworm's mirrors for
`git` and `ca-certificates` in both images and `openssh-client` in the server,
and `registry.npmjs.org` for the harnesses in `HARNESSES`.

**Installing** reaches nothing more with compose, which runs what it built. On
Kubernetes the kubelet pulls the server, the runtime and the CLI from the
registries the values name, and the runtime pulls the runner every time a pod
starts.

**Running** reaches what AIRGAP.md's S1 to S8 and J1 to J7 name: the identity
provider if one is set, the model upstreams the proxy holds credentials for,
the git hosts of the jobs' repositories, and the hosts `HAAS_ALLOWED_HOSTS`
names. A harness calling home from a sealed job is refused and written down
(J1 to J5). With no model upstream set there is no proxy, and a job keeps the
runtime's default network (J7): on these installs that is the Docker bridge,
which reaches anything the machine does.

**It does not reach** anything of ours. Nothing in haas calls a licence server,
sends telemetry, or checks for updates, and the pages load nothing from outside
the server. Nothing needs to reach in but the port the pages are on: the proxy
listens on a unix socket, in the data directory in compose and in a volume only
the pod shares on Kubernetes, and the chart's Service exposes the pages alone.

## What has been run, and what has not

Run on 2026-09-27 on a Linux x86-64 machine, as root, with Docker Engine 29.3.1
and Compose v5.1.1. That machine re-signs outbound TLS with its own CA, so for
the builds its CA was added to the same pinned bases through a compose file kept
outside the repository; nothing under `deploy/` changed for it.

- **Compose.** Both images built. `docker compose up -d --build` brought the
  runner up and down with exit 0 and the server up and healthy. The pages
  answered: the sign-in page, sign-in with the password, the catalog offering
  opencode, `/jobs`, and `/static/htmx.min.js`. `smoke.sh` passed with no
  upstream (the job on the Docker bridge), with an upstream set (the job
  sealed), and with a signing key, when `haas verify` named the signer. A job
  that tried one request through the proxy and one around it, sealed, got the
  proxy's 403 with a `denied` line in `out/proxy.jsonl`, and `Could not resolve
  host`.
- **The chart.** `helm lint --strict` and `helm template` with
  `helm/haas/ci/values.yaml`, and `kubeconform -strict` against Kubernetes
  1.30.0: five resources, all valid. It refuses a tag in place of a digest, a
  missing password, a setting the chart owns, and Kubernetes before 1.29.
- **The chart, on a cluster.** The `install-check` workflow
  (`.github/workflows/install-check.yml`, run 36531022679 on 2026-09-29) checks
  the signatures of v0.1.0-rc.1's two images with cosign, then installs the chart
  from their digests on a `kind` cluster with a pull secret. The pod became
  ready, the server container ran the named digest, sign-in answered 303 and the
  home page 200. The same run installed compose from the digests with
  `--no-build` and passed the same checks. Neither ran a job or reached a model.
- **The chart's pod, without Kubernetes.** kind could not start a pod on that
  machine (runc failed one level down), so the pod was laid out by hand with
  Docker: the runtime image privileged with the chart's arguments, the server
  image beside it, the same volumes at the same paths and the same
  environment. The runtime listened on no TCP port, `smoke.sh`'s job passed
  sealed, and the same two requests were refused and found no route.

- **Compose, cold, by a stranger.** An agent with no other context cloned the
  repository and followed `compose/README.md` alone, on linux/arm64 in the
  Rancher Desktop VM on an Apple Silicon Mac (2026-09-29). Both images built in
  48 s, the server read `healthy` about 15 s after `up`, the smoke check passed
  in 2.2 s, and sign-in and the pages answered. It found eight gaps in the
  README, which were corrected. It ran no model and did not try TLS, signing or
  podman.

Not run yet:

- the chart on a cluster you run, and its ingress: the `install-check` workflow
  installs it on a `kind` cluster, with the ingress off;
- compose on podman's socket, rootful or rootless;
- a real harness with a real model through either install;
- the chart on arm64;
- the chart, on a clean machine, by somebody who did not write it.
