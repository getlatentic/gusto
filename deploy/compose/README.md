# gusto with Compose

The whole product on one Linux machine: the pages, the API and the proxy in one
server, and a container of its own for every job. [../README.md](../README.md)
says what the server is given to start those containers with. Read
[How a job gets its box](../README.md#how-a-job-gets-its-box) before step 1:
**the server holds the host's container runtime socket, which is root on this
machine.**

`deploy/install/server.sh` does what this page describes in one command, and adds HTTPS: it runs the `tls` profile of `compose.yaml`, which puts Caddy in front of the server with a certificate for your domain. Read on for what it does and what the machine needs.

## Behind your own web server

If this machine already runs nginx or Apache, keep it and put gusto behind it. Run `deploy/install/server.sh --behind-proxy`, which keeps gusto on
`127.0.0.1:8040` and tells it a proxy is in front (`GUSTO_BEHIND_PROXY=1`, so the address your web server forwards is the one the sign-in lock and
the record use). Two things matter to gusto whatever the server: a **turn streams** as it goes, so the proxy must not buffer the reply or cut it
off, and gusto must be told the **scheme** so its cookie is marked secure.

nginx:

```nginx
server {
    server_name agents.example.com;
    location / {
        proxy_pass http://127.0.0.1:8040;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;          # a turn streams
        proxy_read_timeout 1h;        # and a stream may be quiet for a while
    }
}
```

then `certbot --nginx -d agents.example.com` for the certificate.

Apache (`a2enmod proxy proxy_http headers`):

```apache
<VirtualHost *:443>
    ServerName agents.example.com
    # certificate lines as certbot --apache writes them
    ProxyPreserveHost On
    ProxyPass        / http://127.0.0.1:8040/ flushpackets=on timeout=3600
    ProxyPassReverse / http://127.0.0.1:8040/
    RequestHeader set X-Forwarded-Proto "https"
</VirtualHost>
```

Apache adds `X-Forwarded-For` itself. Anything else that terminates TLS in front (Cloudflare, a load balancer, Tailscale Serve) works the same way:
send it to `http://127.0.0.1:8040`, do not buffer, and pass the scheme and the client's address.

## What the machine needs

- Linux on x86-64 is the target. On a GitHub-hosted Ubuntu runner (AMD EPYC, 2 vCPU,
  7 GiB, kernel 6.17.0-1022-azure), one run each, building both images from source
  took 83 s and the server read `healthy` 11 s after `up`. Installing a release from
  its digests took 80 s to pull both images and 6 s from `create` until `/healthz`
  answered.
- Both images also built and ran on linux/arm64, in the Rancher Desktop VM on an
  Apple Silicon Mac. That cold install measured 1.3 s to clone, 48 s to build both
  images, and about 15 s from `up` until the server read `healthy`. On a Mac the
  engine is a VM, so `GUSTO_DATA` has to be a path the VM shares with the Mac. No
  other host has been tried.
- Docker Engine with the Compose plugin, v2.24 or later, which is the first to
  read the `env_file` form `compose.yaml` uses. `docker compose version` says.
- Nothing else of value on it, for the reason above.
- Outbound, while it builds: `docker.io`, `proxy.golang.org`, Debian's mirrors
  and `registry.npmjs.org`. [../README.md](../README.md#what-it-needs-outbound-and-what-it-does-not)
  has the rest.

Run the commands as root or as a user in the `docker` group.

## Install

To connect the desktop app's helper (`gusto-helper login`) to this server, also turn on `GUSTO_API_TOKENS` ([../../docs/TOKENS.md](../../docs/TOKENS.md)). With no identity provider, the shared password is the one owner and approves the helper's code.

With this repository on the machine, at the commit you mean to run:

```bash
cd gusto/deploy/compose
sed "s|^GUSTO_PASSWORD=.*|GUSTO_PASSWORD=$(openssl rand -hex 24)|" env.example > .env
grep ^GUSTO_PASSWORD .env      # you sign in with this
```

That makes `.env` from `env.example` with a password in it. The server will not
start without one. Two more settings in `.env` are worth a look before you
build: `GUSTO_DATA` is where jobs and conversations live on the host, and Compose
creates it if it is not there; `GUSTO_PUBLISH` is the address and port the pages
answer on.

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
why it does not offer the rest: each line that begins `not offered` names a
harness the runner image does not carry, and a harness it does not name is
offered, which with the default `.env` is `opencode`. Those lines say `HARNESSES`
where `.env` says `GUSTO_RUNNER_HARNESSES`.

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

The pages are on `127.0.0.1:8040`, or where `GUSTO_PUBLISH` in `.env` puts them.
From another machine, forward the port:

```bash
ssh -L 8040:127.0.0.1:8040 <this machine>
```

and open <http://127.0.0.1:8040>. With no browser, `curl` does the same:

```bash
curl -s -c jar -o /dev/null -w '%{http_code}\n' \
  --data-urlencode "password=<the password>" http://127.0.0.1:8040/login   # 303
curl -s -b jar -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8040/    # 200
```

A right password answers 303 to `/` and sets the `gusto` cookie. A wrong one
answers 303 to `/login?wrong=1`. `/jobs` and `/static/htmx.min.js` answer 200
signed in, and `/new/blank` lists `OpenCode`.

Before anybody signs in from anywhere else,
put something that terminates TLS in front and set `GUSTO_PUBLISH` to where it
reaches the server. The password is sent in the clear otherwise. A job's
output is a stream that stays open while the job runs, so the proxy in front
must not buffer `/watch/` or time it out early:
`../ansible/roles/nginx/templates/gusto.conf.j2` is nginx doing it.

## Give it a model

A harness needs a model to be any use. In `.env`:

```bash
GUSTO_MODEL_UPSTREAM=https://models.example.com   # the root, with no version on it
GUSTO_MODEL_KEY=...
GUSTO_MODELS=some-model
```

then `docker compose up -d`. With an upstream set, the proxy is on: every job
runs with no network but the proxy's socket, and the key stays in the server.
Without one, a job keeps the Docker bridge and reaches anything this machine
does. `GUSTO_ALLOWED_HOSTS` names the other hosts a sealed job may reach.

Every other setting is in the Environment table of the
[root README](../../docs/REFERENCE.md#environment), and goes in `.env`.

A harness this image does not carry is added to `GUSTO_RUNNER_HARNESSES`, as
`name=package`, and built with `docker compose up -d --build`.

## Sign the record

```bash
docker compose exec server gusto keygen <name> -dir /var/lib/gusto/keys
```

then in `.env`, with `GUSTO_DATA` in place of `/var/lib/gusto` if you moved it:

```bash
GUSTO_SIGNING_KEY=/var/lib/gusto/keys/<name>.key
GUSTO_KEYRING=/var/lib/gusto/keys
```

and `docker compose up -d`. The key stays in the server's container: a job is
given its own directory, never this one. The smoke check then prints
`signed by <name>`.

## Where things are

| what | where |
| --- | --- |
| jobs and their evidence | `$GUSTO_DATA/jobs` on the host, `/var/lib/gusto/jobs` unless moved |
| conversations' workspaces | `$GUSTO_DATA/sessions` |
| harnesses' setups | `$GUSTO_DATA/setups` |
| comparisons | `$GUSTO_DATA/comparisons` |
| the record of what changed on the platform | `$GUSTO_DATA/changes/changes.jsonl`; [../../docs/CHANGES.md](../../docs/CHANGES.md) |
| the proxy's socket, with an upstream set | `$GUSTO_DATA/jobs/.proxy.sock` |
| the smoke check's repository | `$GUSTO_DATA/smoke`; its jobs sit with the others in `$GUSTO_DATA/jobs`, and stay until you delete them |
| the server's log | `docker compose logs server`: one JSON line a request |

`GUSTO_DATA` is mounted at the same path inside the server as on the host, and
has to be. The server names a job's directory to the runtime by the path it
sees, and the runtime looks for that path on the host.

Backing up is copying `GUSTO_DATA`.

## Upgrade, stop, remove

```bash
git pull && docker compose up -d --build     # upgrade
docker compose down                          # stop; GUSTO_DATA stays
```

Compose names the project `gusto`, from `name:` in `compose.yaml`. Set
`COMPOSE_PROJECT_NAME` to run a second install on the same machine.

## Podman

For rootful podman, `systemctl enable --now podman.socket` and set
`GUSTO_RUNTIME_SOCKET=/run/podman/podman.sock`. The `docker` CLI in the server
speaks podman's Docker-compatible API, and the jobs' paths are the host's either
way. This has not been run yet, and rootless podman has not been tried at all.
