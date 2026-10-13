# A pilot on your own infrastructure

What a client provides, the steps to install a release, and the checks to run
before anybody signs in. Every command here is one the install check runs on
each release (`.github/workflows/install-check.yml`), unless a line says it is
not.

## What the client provides

| Item | Needed for |
| --- | --- |
| A Linux x86-64 machine with Docker Engine and the Compose plugin v2.24 or later, or a Kubernetes cluster | the install. [deploy/compose/README.md](../deploy/compose/README.md) and [deploy/helm/haas/README.md](../deploy/helm/haas/README.md) say what each needs |
| Something that terminates TLS in front of the server | any sign-in from another machine. The password travels in the clear otherwise |
| Outbound access to `ghcr.io` and to the model provider | pulling the images, and the model calls. [deploy/README.md](../deploy/README.md#what-it-needs-outbound-and-what-it-does-not) lists the rest |
| A token that can read the packages in `ghcr.io/getlatentic` | pulling the images. The repository is private, so its images are too |
| A model provider's address and key | a harness that asks a model |
| An identity provider, or a shared password | sign-in. [SSO.md](SSO.md) says what to set at the provider and on the server |

The server holds the host's container runtime socket, which is root on the
machine. Put nothing else of value on it ([deploy/README.md](../deploy/README.md#how-a-job-gets-its-box)).

## Install a release

A release names its two images by digest in its notes. Check the signature,
then install from the digests, with no build:

```bash
cosign verify ghcr.io/getlatentic/haas-server@sha256:<digest> \
  --certificate-identity https://github.com/getlatentic/haas/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
docker login ghcr.io
cd deploy/compose
sed "s|^HAAS_PASSWORD=.*|HAAS_PASSWORD=$(openssl rand -hex 24)|" env.example > .env
```

Add `HAAS_SERVER_IMAGE` and `HAAS_RUNNER_IMAGE` to `.env`, as the release's notes
give them, then:

```bash
docker compose pull
docker compose up -d --no-build
curl -fsS http://127.0.0.1:8040/healthz        # ok
```

The chart takes the same digests. [deploy/helm/haas/README.md](../deploy/helm/haas/README.md#images) has the commands.

## Check it before handing it over

| Check | Command | Passes when |
| --- | --- | --- |
| The server runs the release | `docker inspect --format '{{.Config.Image}}' "$(docker compose ps -q server)"` | it prints the server digest the release names |
| It signs in | `curl -s -o /dev/null -w '%{http_code}' --data-urlencode "password=<the password>" http://127.0.0.1:8040/login` | `303` |
| It runs a job and keeps a record | `docker compose exec -T server sh < ../smoke.sh` | the last line says the gate still exits 0, and the job page named on a failure says why |
| It counts a model call | submit a job on a real model, then read `$HAAS_DATA/jobs/<id>/out/proxy.jsonl` | the line has `inputTokens`, `outputTokens` and a `usd`. A model with no price in `HAAS_SPEND_POLICY` has none, and that is unmeasured, not free |
| It signs people in through your provider | set the `HAAS_OIDC_*` settings and a group ([SSO.md](SSO.md)), then sign in from a browser | a person in an admin group sees the admin pages, and one in no group is sent back to the login page with a refusal |
| It isolates a job | `docker compose exec -T server sh < ../smoke-isolation.sh` | it ends by saying the box has no network but loopback, no runtime socket, none of the server's data, and cannot gain a privilege |
| It signs the record | set `HAAS_SIGNING_KEY` ([the Compose README](../deploy/compose/README.md#sign-the-record)), run the job again | the check prints `signed by <name>` |

The install check runs the first three, the isolation probe, and the sign-in through Dex, on every release, against a stub model
for the counting. A stub shows that the proxy holds the key, reaches the model
and counts the call. It says nothing about the client's model.

## Check these with your own setup

The install check runs against stand-ins. These three are the client's own, so run each once before the handover:

- **Your identity provider.** The install check signs in through Dex, not through a company's own provider. Try yours
  ([SSO.md](SSO.md)).
- **Your model.** The install check uses a stub. Run one real job on the client's model and read its record.
- **Your isolation needs.** Jobs run in Docker containers with no network but the proxy's socket, which share the host's kernel.
  [SECURITY.md](SECURITY.md#how-a-job-is-isolated) says what was probed. Where a client wants a stronger wall, run each job in an
  OpenShell microVM on a host with KVM ([OPENSHELL.md](OPENSHELL.md)).

Podman, rootless Docker and hosts other than Linux on x86-64 and arm64 are on the [roadmap](ROADMAP.md).

## After the handover

- **Back up** by copying `HAAS_DATA`.
- **Upgrade** by installing the next release's digests the same way. Read the
  release's section in [RELEASES.md](RELEASES.md) first: it lists the settings
  added and what is refused now. Rehearse it on a copy of the client's data with
  `deploy/rehearse-upgrade.sh <new haas binary> <HAAS_DATA>`, which never changes the
  original. [UPGRADE.md](UPGRADE.md) says what it migrates, and how to go back with
  `haas data rollback -to <release>`.
- **Watch** `/healthz`, `/readyz` and `/metrics`, and the alerts in
  [RUNBOOK.md](RUNBOOK.md).
- **Keep** finished work for as long as the client's policy says
  ([RETENTION.md](RETENTION.md)).
