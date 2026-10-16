# gusto on Kubernetes

The whole product in one pod: the pages, the API and the proxy in one server, and a container for every job. Read [How a job gets its box](../../README.md#how-a-job-gets-its-box) first: **the job runtime in this pod is privileged, which is root on the node that runs it**, and the namespace has to admit that.

The `install-check` workflow installs the chart on a `kind` cluster from a release's digests. It has not been installed on a cluster you run, and arm64 has not been tried ([what has been run](../../README.md#what-has-been-run-and-what-has-not)).

## Requirements

- Kubernetes 1.29 or later (the chart refuses older) and a namespace whose Pod Security admits a privileged pod.
- A default storage class, or `persistence.storageClass`, or your own claim in `persistence.existingClaim`.
- x86-64 nodes, and a registry the cluster pulls from, holding the two images below.

## Images

A release names the server and the runner by digest, signed, in its notes. [Check the signatures](../../README.md#published-images) before you pull. The images are private. The kubelet pulls the server and the daemon in the pod pulls the runner, and one `kubernetes.io/dockerconfigjson` Secret serves both:

```bash
kubectl -n gusto create secret docker-registry ghcr \
  --docker-server=ghcr.io --docker-username=<user> --docker-password=<token>
helm install gusto deploy/helm/gusto -n gusto \
  --set image.server.repository=ghcr.io/getlatentic/gusto-server --set image.server.digest=sha256:<digest> \
  --set image.runner.repository=ghcr.io/getlatentic/gusto-runner --set image.runner.digest=sha256:<digest> \
  --set 'imagePullSecrets[0].name=ghcr' --set runnerPullSecret=ghcr \
  --set auth.password=<password>
```

To use your own registry instead, build with Compose, from the repository root on a machine with Docker, so the images use the same pinned bases as the Compose install. Push both and give the chart their digests:

```bash
cd deploy/compose
cp env.example .env          # compose reads it even to build
export GUSTO_SERVER_IMAGE=registry.example.com/gusto-server:$(git rev-parse --short HEAD)
export GUSTO_RUNNER_IMAGE=registry.example.com/gusto-runner:$(git rev-parse --short HEAD)
docker compose build
docker compose push
server_digest=$(docker buildx imagetools inspect "$GUSTO_SERVER_IMAGE" --format '{{.Manifest.Digest}}')
runner_digest=$(docker buildx imagetools inspect "$GUSTO_RUNNER_IMAGE" --format '{{.Manifest.Digest}}')
cd ../..
```

`GUSTO_RUNNER_HARNESSES` in `.env` lists the harnesses the runner carries, as `name=package`. The chart takes the digest and refuses a tag. The runtime pulls the runner, not the kubelet, so `imagePullSecrets` does not reach it: for a registry that needs a sign-in, name a `kubernetes.io/dockerconfigjson` Secret in `runnerPullSecret`.

## Install

```bash
kubectl create namespace gusto
kubectl label namespace gusto pod-security.kubernetes.io/enforce=privileged
password=$(openssl rand -hex 24)
kubectl -n gusto create secret generic gusto-auth --from-literal=GUSTO_PASSWORD="$password"
echo "$password"             # you sign in with this
helm install gusto deploy/helm/gusto -n gusto \
  --set image.server.repository="${GUSTO_SERVER_IMAGE%:*}" \
  --set image.server.digest="$server_digest" \
  --set image.runner.repository="${GUSTO_RUNNER_IMAGE%:*}" \
  --set image.runner.digest="$runner_digest" \
  --set auth.existingSecret=gusto-auth
kubectl -n gusto rollout status deploy/gusto
```

The pod starts the runtime, then pulls the runner into it, then starts the server. `kubectl -n gusto logs deploy/gusto -c server` says which harnesses it offers, and why it does not offer the rest. Every job runs inside the runtime's container, so the scheduler sees one container. Give `resources.runtime` at least `jobs.concurrency` times `jobs.memory`, or the node kills jobs the scheduler never counted.

## Prove it runs a job

```bash
# from the repository root
kubectl -n gusto exec -i deploy/gusto -c server -- sh < deploy/smoke.sh
```

It runs one job through the API on a fixture harness that calls no model, and ends with `unsigned: no key stands behind this record` and `8 files match, and the gate still exits 0`.

## Sign in

Run `kubectl -n gusto port-forward svc/gusto 8040:80` and open <http://127.0.0.1:8040>. To serve a name, turn on `ingress` with TLS. The ingress must not buffer a job's output or time it out early. With ingress-nginx:

```yaml
ingress:
  enabled: true
  className: nginx
  host: gusto.example.com
  tls: [{ hosts: [gusto.example.com], secretName: gusto-tls }]
  annotations: { nginx.ingress.kubernetes.io/proxy-buffering: "off", nginx.ingress.kubernetes.io/proxy-read-timeout: "3600" }
```

## Give it a model

```bash
kubectl -n gusto create secret generic gusto-model --from-literal=GUSTO_MODEL_KEY=...
helm upgrade gusto deploy/helm/gusto -n gusto --reuse-values \
  --set model.upstream=https://models.example.com \
  --set model.existingSecret=gusto-model \
  --set model.models=some-model
```

With an upstream set, the proxy is on: every job runs with no network but the proxy's socket, and the key stays in the server. Without one, a job keeps the runtime's network and reaches anything the pod does. Put several credentials in a Secret of `<name>.json` files and name it in `credentials.existingSecret`.

Every other setting in the [Environment table](../../../docs/REFERENCE.md#environment) goes under `env` as `GUSTO_NAME: value`. The chart refuses the ones it owns, because they name the paths and the runtime the pod mounts.

## Where things are

The claim holds `/var/lib/gusto/jobs`, `sessions` and `setups`, and is kept when the chart is uninstalled: the jobs are the record. The proxy's socket is `/run/gusto/proxy/proxy.sock`, in a volume only the pod shares. The runtime's images live in the pod's storage and go with the pod.
