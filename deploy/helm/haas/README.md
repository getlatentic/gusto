# haas on Kubernetes

The whole product in one pod: the pages, the API and the proxy in one server,
and a container of its own for every job. [deploy/README.md](../../README.md)
says what the server is given to start those containers with. Read
[How a job gets its box](../../README.md#how-a-job-gets-its-box) before
installing: **the job runtime in this pod is privileged, which is root on the
node that runs it**, and the namespace has to admit that.

This chart has been linted, rendered and checked against the Kubernetes schema,
and its pod has been laid out by hand with Docker. It has not yet been installed
on a cluster. [What has been run](../../README.md#what-has-been-run-and-what-has-not)
says exactly what was run.

## What the cluster needs

- Kubernetes 1.29 or later, for the sidecar that starts the runtime before the
  server. The chart refuses older.
- A namespace whose Pod Security admits a privileged pod.
- A default storage class, or `persistence.storageClass`, or a claim of your
  own in `persistence.existingClaim`.
- x86-64 nodes. arm64 has not been tried.
- A registry the cluster pulls from, holding the two images below.

## Images

No public image exists for the server or the runner, so you build both, push
them, and give the chart their digests. Compose builds them from the same pinned
bases as the compose install, so build with it, from the repository root, on a
machine with Docker:

```bash
cd deploy/compose
cp env.example .env          # compose reads it even to build
export HAAS_SERVER_IMAGE=registry.example.com/haas-server:$(git rev-parse --short HEAD)
export HAAS_RUNNER_IMAGE=registry.example.com/haas-runner:$(git rev-parse --short HEAD)
docker compose build
docker compose push
server_digest=$(docker buildx imagetools inspect "$HAAS_SERVER_IMAGE" --format '{{.Manifest.Digest}}')
runner_digest=$(docker buildx imagetools inspect "$HAAS_RUNNER_IMAGE" --format '{{.Manifest.Digest}}')
cd ../..
```

`HAAS_RUNNER_HARNESSES` in `.env` is the list of harnesses the runner carries,
as `name=package`. The tag is for people; the chart takes the digest and
refuses a tag in its place.

The runtime pulls the runner itself, not the kubelet, so `imagePullSecrets`
does not reach it. For a registry that needs a sign-in, make a
`kubernetes.io/dockerconfigjson` Secret and name it in `runnerPullSecret`.

## Install

```bash
kubectl create namespace haas
kubectl label namespace haas pod-security.kubernetes.io/enforce=privileged

password=$(openssl rand -hex 24)
kubectl -n haas create secret generic haas-auth --from-literal=HAAS_PASSWORD="$password"
echo "$password"             # you sign in with this

helm install haas deploy/helm/haas -n haas \
  --set image.server.repository="${HAAS_SERVER_IMAGE%:*}" \
  --set image.server.digest="$server_digest" \
  --set image.runner.repository="${HAAS_RUNNER_IMAGE%:*}" \
  --set image.runner.digest="$runner_digest" \
  --set auth.existingSecret=haas-auth
kubectl -n haas rollout status deploy/haas
```

The pod starts in order: the runtime, then the runner pulled into it, then the
server. `kubectl -n haas logs deploy/haas -c server` says which harnesses it
offers, and why it does not offer the rest.

## Prove it runs a job

From the repository root:

```bash
kubectl -n haas exec -i deploy/haas -c server -- sh < deploy/smoke.sh
```

It runs one job through the server's API, in a container of its own, on a
fixture harness that makes the change and calls no model, and then checks the
evidence. It ends with:

```
unsigned: no key stands behind this record
8 files match, and the gate still exits 0
```

## Sign in

```bash
kubectl -n haas port-forward svc/haas 8040:80
```

and open <http://127.0.0.1:8040>. To serve it on a name, turn on `ingress` with
TLS. A job's output is a stream that stays open while the job runs, so the
ingress must not buffer it or time it out early. With ingress-nginx:

```yaml
ingress:
  enabled: true
  className: nginx
  host: haas.example.com
  tls: [{ hosts: [haas.example.com], secretName: haas-tls }]
  annotations:
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
```

## Give it a model

```bash
kubectl -n haas create secret generic haas-model --from-literal=HAAS_MODEL_KEY=...
helm upgrade haas deploy/helm/haas -n haas --reuse-values \
  --set model.upstream=https://models.example.com \
  --set model.existingSecret=haas-model \
  --set model.models=some-model
```

With an upstream set, the proxy is on: every job runs with no network but the
proxy's socket, and the key stays in the server. Without one, a job keeps the
runtime's own network and reaches anything the pod does. Several credentials
go in a Secret of `<name>.json` files, named in `credentials.existingSecret`.

Every other setting in the [root README's Environment table](../../../README.md#environment)
goes under `env`, as `HAAS_NAME: value`. The chart refuses the ones it owns,
because they name the paths and the runtime the pod mounts.

## What one job may take

Every job runs inside the runtime's container, so the scheduler sees one
container for all of them. Give `resources.runtime` at least
`jobs.concurrency` times `jobs.memory`, or the node kills jobs the scheduler
never counted.

## Where things are

| what | where |
| --- | --- |
| jobs and their evidence | the claim, at `/var/lib/haas/jobs` |
| conversations' workspaces | the claim, at `/var/lib/haas/sessions` |
| harnesses' setups | the claim, at `/var/lib/haas/setups` |
| the proxy's socket | `/run/haas/proxy/proxy.sock`, in a volume only the pod shares |
| the runtime's images | the pod's own storage, gone with the pod |

The claim is kept when the chart is uninstalled: the jobs are the record.
