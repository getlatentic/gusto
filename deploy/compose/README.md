# haas with Compose

The whole product on one Linux machine: the pages, the API and the proxy in one
server, and a container of its own for every job. [../README.md](../README.md)
says what the server is given to start those containers with. Read
[How a job gets its box](../README.md#how-a-job-gets-its-box) before step 1:
**the server holds the host's container runtime socket, which is root on this
machine.**

## What the machine needs

- Linux on x86-64. arm64 has not been tried.
- Docker Engine with the Compose plugin, v2.24 or later, which is the first to
  read the `env_file` form `compose.yaml` uses. `docker compose version` says.
- Nothing else of value on it, for the reason above.
- Outbound, while it builds: `docker.io`, `proxy.golang.org`, Debian's mirrors
  and `registry.npmjs.org`. [../README.md](../README.md#what-it-needs-outbound-and-what-it-does-not)
  has the rest.

Run the commands as root or as a user in the `docker` group.

## Install

With this repository on the machine, at the commit you mean to run:

```bash
cd haas/deploy/compose
cp env.example .env
```

Set the password in `.env`. The server will not start without one.

```bash
sed -i "s|^HAAS_PASSWORD=.*|HAAS_PASSWORD=$(openssl rand -hex 24)|" .env
grep ^HAAS_PASSWORD .env      # you sign in with this
```

Build and start. The first build downloads the harnesses from npm and takes a
while.

```bash
docker compose up -d --build
docker compose ps -a
```

`runner` has to read `Exited (0)` and `server` has to read `healthy`. The runner
is not meant to stay up: compose builds its image and runs it once to prove it
starts, and from then on the server starts a fresh container from it for every
job. `docker compose logs server` says which harnesses the server offers, and
why it does not offer the rest.

## Prove it runs a job

```bash
docker compose exec -T server sh < ../smoke.sh
```

It runs one job through the server's API, in a container of its own, on a
fixture harness that makes the change and calls no model, and then checks the
evidence. It ends with:

```
unsigned: no key stands behind this record
8 files match, and the gate still exits 0
```

Anything else is a failure, and the job page it names says why. A pass proves
the install, and says nothing about a model.

## Sign in

The pages are on `127.0.0.1:8040`. From another machine, forward the port:

```bash
ssh -L 8040:127.0.0.1:8040 <this machine>
```

and open <http://127.0.0.1:8040>. Before anybody signs in from anywhere else,
put something that terminates TLS in front and set `HAAS_PUBLISH` to where it
reaches the server. The password is sent in the clear otherwise. A job's
output is a stream that stays open while the job runs, so the proxy in front
must not buffer `/watch/` or time it out early:
`../ansible/roles/nginx/templates/haas.conf.j2` is nginx doing it.

## Give it a model

A harness needs a model to be any use. In `.env`:

```bash
HAAS_MODEL_UPSTREAM=https://models.example.com   # the root, with no version on it
HAAS_MODEL_KEY=...
HAAS_MODELS=some-model
```

then `docker compose up -d`. With an upstream set, the proxy is on: every job
runs with no network but the proxy's socket, and the key stays in the server.
Without one, a job keeps the Docker bridge and reaches anything this machine
does. `HAAS_ALLOWED_HOSTS` names the other hosts a sealed job may reach.

Every other setting is in the Environment table of the
[root README](../../README.md#environment), and goes in `.env`.

A harness this image does not carry is added to `HAAS_RUNNER_HARNESSES`, as
`name=package`, and built with `docker compose up -d --build`.

## Sign the record

```bash
docker compose exec server haas keygen <name> -dir /var/lib/haas/keys
```

then in `.env`, with `HAAS_DATA` in place of `/var/lib/haas` if you moved it:

```bash
HAAS_SIGNING_KEY=/var/lib/haas/keys/<name>.key
HAAS_KEYRING=/var/lib/haas/keys
```

and `docker compose up -d`. The key stays in the server's container: a job is
given its own directory, never this one. The smoke check then prints
`signed by <name>`.

## Where things are

| what | where |
| --- | --- |
| jobs and their evidence | `$HAAS_DATA/jobs` on the host, `/var/lib/haas/jobs` unless moved |
| conversations' workspaces | `$HAAS_DATA/sessions` |
| the proxy's socket, with an upstream set | `$HAAS_DATA/jobs/.proxy.sock` |
| the server's log | `docker compose logs server`: one JSON line a request |

`HAAS_DATA` is mounted at the same path inside the server as on the host, and
has to be. The server names a job's directory to the runtime by the path it
sees, and the runtime looks for that path on the host.

Backing up is copying `HAAS_DATA`.

## Upgrade, stop, remove

```bash
git pull && docker compose up -d --build     # upgrade
docker compose down                          # stop; HAAS_DATA stays
```

## Podman

For rootful podman, `systemctl enable --now podman.socket` and set
`HAAS_RUNTIME_SOCKET=/run/podman/podman.sock`. The `docker` CLI in the server
speaks podman's Docker-compatible API, and the jobs' paths are the host's either
way. This has not been run yet, and rootless podman has not been tried at all.
