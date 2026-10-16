# A pilot on your own infrastructure

Install a release, then run the checks before anybody signs in. The install check (`.github/workflows/install-check.yml`) runs these commands on each release.

## What the client provides

| Item | Needed for |
| --- | --- |
| A Linux x86-64 machine with Docker Engine and the Compose plugin v2.24 or later, or a Kubernetes cluster | the install. See [deploy/compose/README.md](../deploy/compose/README.md) and [deploy/helm/gusto/README.md](../deploy/helm/gusto/README.md) |
| Something that terminates TLS in front of the server | any sign-in from another machine. The password travels in the clear otherwise |
| Outbound access to `ghcr.io` and to the model provider | pulling the images, and the model calls. [deploy/README.md](../deploy/README.md#what-it-needs-outbound-and-what-it-does-not) lists the rest |
| A token that can read the packages in `ghcr.io/getlatentic` | pulling the images. The images are private |
| A model provider's address and key | a harness that asks a model |
| An identity provider, or a shared password | sign-in. See [SSO.md](SSO.md) |

The server holds the host's container runtime socket, which is root on the machine. Put nothing else of value on it ([deploy/README.md](../deploy/README.md#how-a-job-gets-its-box)).
## Install a release

The release notes name the two images by digest. Verify the signature, then install with no build:

```bash
cosign verify ghcr.io/getlatentic/gusto-server@sha256:<digest> \
  --certificate-identity https://github.com/getlatentic/gusto/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
docker login ghcr.io
cd deploy/compose
sed "s|^GUSTO_PASSWORD=.*|GUSTO_PASSWORD=$(openssl rand -hex 24)|" env.example > .env
```

Add `GUSTO_SERVER_IMAGE` and `GUSTO_RUNNER_IMAGE` to `.env`, as the release notes give them. Then run:
```bash
docker compose pull
docker compose up -d --no-build
curl -fsS http://127.0.0.1:8040/healthz        # ok
```
The chart takes the same digests: [deploy/helm/gusto/README.md](../deploy/helm/gusto/README.md#images).

## Check it before handover

| Check | Command | Passes when |
| --- | --- | --- |
| The server runs the release | `docker inspect --format '{{.Config.Image}}' "$(docker compose ps -q server)"` | it prints the server digest the release names |
| It signs in | `curl -s -o /dev/null -w '%{http_code}' --data-urlencode "password=<the password>" http://127.0.0.1:8040/login` | `303` |
| It runs a job and keeps a record | `docker compose exec -T server sh < ../smoke.sh` | the last line says the gate still exits 0 |
| It counts a model call | submit a job on a real model, then read `$GUSTO_DATA/jobs/<id>/out/proxy.jsonl` | the line has `inputTokens`, `outputTokens` and a `usd`. A model with no price in `GUSTO_SPEND_POLICY` has none, and that means unmeasured, not free |
| It signs people in through your provider | set the `GUSTO_OIDC_*` settings and a group ([SSO.md](SSO.md)), then sign in from a browser | a person in an admin group sees the admin pages, and a person in no group returns to the login page with a refusal |
| It isolates a job | `docker compose exec -T server sh < ../smoke-isolation.sh` | it ends by saying the box has no network but loopback, no runtime socket, none of the server's data, and cannot gain a privilege |
| It signs the record | set `GUSTO_SIGNING_KEY` ([Compose README](../deploy/compose/README.md#sign-the-record)), run the job again | the check prints `signed by <name>` |

The install check uses a stub model and Dex, so run these three with the client's own setup once before handover:
- Your identity provider: sign in through it ([SSO.md](SSO.md)).
- Your model: run one real job and read its record.
- Your isolation needs: jobs run in Docker containers that share the host's kernel and have no network but the proxy's socket ([SECURITY.md](SECURITY.md#how-a-job-is-isolated)). For a stronger wall, run each job in an OpenShell microVM on a host with KVM ([OPENSHELL.md](OPENSHELL.md)).

## After handover

- Back up by copying `GUSTO_DATA`.
- Upgrade by installing the next release's digests. Read the release's section in [RELEASES.md](RELEASES.md) first. Rehearse on a copy of the data with `deploy/rehearse-upgrade.sh <new gusto binary> <GUSTO_DATA>`, which never changes the original. To go back, run `gusto data rollback -to <release>` ([UPGRADE.md](UPGRADE.md)).
- Watch `/healthz`, `/readyz` and `/metrics`, and the alerts in [RUNBOOK.md](RUNBOOK.md).
- Keep finished work as long as the client's policy says ([RETENTION.md](RETENTION.md)).
