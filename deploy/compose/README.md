# gusto with Compose

The whole product on one Linux machine: the pages, the API and the proxy in one server, and a container for every job. Read [How a job gets its box](../README.md#how-a-job-gets-its-box) first: **the server holds the host's container runtime socket, which is root on this machine.** Put nothing else of value on the machine.

`deploy/install/server.sh` does what this page describes in one command and adds HTTPS: the `tls` profile of `compose.yaml` puts Caddy in front of the server with a certificate for your domain.

## Requirements

- Linux on x86-64. linux/arm64 also built and ran, in the Rancher Desktop VM on an Apple Silicon Mac, where `GUSTO_DATA` has to be a path the VM shares with the Mac. No other host has been tried.
- Docker Engine with the Compose plugin v2.24 or later (`docker compose version`). Run the commands as root or as a user in the `docker` group.
- Outbound while it builds: `docker.io`, `proxy.golang.org`, Debian's mirrors and `registry.npmjs.org` ([more](../README.md#what-it-needs-outbound-and-what-it-does-not)).

## Install

From this repository, at the commit you mean to run:

```bash
cd gusto/deploy/compose
sed "s|^GUSTO_PASSWORD=.*|GUSTO_PASSWORD=$(openssl rand -hex 24)|" env.example > .env
grep ^GUSTO_PASSWORD .env      # you sign in with this
docker compose up -d --build   # the first build downloads the harnesses and takes a while
docker compose ps -a
```

The server will not start without a password. In `.env`, `GUSTO_DATA` is where jobs and conversations live on the host (Compose creates it) and `GUSTO_PUBLISH` is the address and port of the pages. To connect the desktop helper (`gusto-helper login`), also set `GUSTO_API_TOKENS` ([TOKENS.md](../../docs/TOKENS.md)).

`runner` has to read `Exited (0)` and `server` has to read `healthy`. The runner runs once to prove its image starts, and the server starts a fresh container from it for every job. `docker compose logs server` lists each harness it does not offer, on lines that begin `not offered`. The default `.env` offers `opencode`. Those lines say `HARNESSES` where `.env` says `GUSTO_RUNNER_HARNESSES`.

## Prove it runs a job

```bash
docker compose exec -T server sh < ../smoke.sh
```

It runs one job through the API on a fixture harness that calls no model, and checks the evidence. It ends with:

```
unsigned: no key stands behind this record
8 files match, and the gate still exits 0
```

Anything else is a failure, and the job page it names says why. A pass says nothing about a model.

## Sign in

The pages are on `127.0.0.1:8040`, or where `GUSTO_PUBLISH` puts them. From another machine, run `ssh -L 8040:127.0.0.1:8040 <this machine>` and open <http://127.0.0.1:8040>. A right password answers 303 to `/` and sets the `gusto` cookie. A wrong one answers 303 to `/login?wrong=1`.

Before anybody signs in from elsewhere, put something that terminates TLS in front and set `GUSTO_PUBLISH` to where it reaches the server. Otherwise the password travels in the clear.

## Behind your own web server

If the machine already runs nginx or Apache, run `deploy/install/server.sh --behind-proxy`. It keeps gusto on `127.0.0.1:8040` and sets `GUSTO_BEHIND_PROXY=1`, so the address your web server forwards is the one the sign-in lock and the record use. The proxy must not buffer or cut off a streaming reply, and must pass the scheme so the cookie is marked secure.

| Need | nginx (`certbot --nginx -d agents.example.com` adds the certificate) | Apache (`a2enmod proxy proxy_http headers`) |
| --- | --- | --- |
| Forward to gusto | `proxy_pass http://127.0.0.1:8040;` and `proxy_http_version 1.1;` | `ProxyPass / http://127.0.0.1:8040/` and `ProxyPassReverse / http://127.0.0.1:8040/` |
| Keep the host | `proxy_set_header Host $host;` | `ProxyPreserveHost On` |
| Pass the client address | `proxy_set_header X-Forwarded-For $remote_addr;` | Apache adds `X-Forwarded-For` itself |
| Pass the scheme | `proxy_set_header X-Forwarded-Proto $scheme;` | `RequestHeader set X-Forwarded-Proto "https"` |
| Stream a turn | `proxy_buffering off;` and `proxy_read_timeout 1h;` | `flushpackets=on timeout=3600` on `ProxyPass` |

Any other TLS front (Cloudflare, a load balancer, Tailscale Serve) works the same way: send to `http://127.0.0.1:8040`, do not buffer, and pass the scheme and the client's address.

## Give it a model

Add to `.env`, then run `docker compose up -d`:

```bash
GUSTO_MODEL_UPSTREAM=https://models.example.com   # the root, with no version on it
GUSTO_MODEL_KEY=...
GUSTO_MODELS=some-model
```

With an upstream set, the proxy is on: every job runs with no network but the proxy's socket, and the key stays in the server. Without one, a job keeps the Docker bridge and reaches anything this machine does. `GUSTO_ALLOWED_HOSTS` names other hosts a sealed job may reach. Every other setting is in the [Environment table](../../docs/REFERENCE.md#environment) and goes in `.env`. Add a harness the image lacks to `GUSTO_RUNNER_HARNESSES` as `name=package`, then run `docker compose up -d --build`.

## Sign the record

Make a key, add it to `.env` (use `GUSTO_DATA` in place of `/var/lib/gusto` if you moved it), and run `docker compose up -d`. The key stays in the server's container, and the smoke check then prints `signed by <name>`.

```bash
docker compose exec server gusto keygen <name> -dir /var/lib/gusto/keys
# in .env:
GUSTO_SIGNING_KEY=/var/lib/gusto/keys/<name>.key
GUSTO_KEYRING=/var/lib/gusto/keys
```

## Where things are

| What | Where |
| --- | --- |
| Jobs and their evidence | `$GUSTO_DATA/jobs` (`/var/lib/gusto/jobs` unless moved); the proxy's socket is `.proxy.sock` there. The smoke check keeps its repository in `$GUSTO_DATA/smoke` and its jobs here until you delete them |
| Workspaces, setups, comparisons | `$GUSTO_DATA/sessions`, `setups`, `comparisons` |
| The change record | `$GUSTO_DATA/changes/changes.jsonl` ([CHANGES.md](../../docs/CHANGES.md)) |
| The server's log | `docker compose logs server`: one JSON line a request |

`GUSTO_DATA` is mounted at the same path inside the server as on the host, and has to be, because the runtime looks for the job's directory on the host. Back up by copying `GUSTO_DATA`.

## Upgrade, stop, remove

Upgrade with `git pull && docker compose up -d --build`. Stop with `docker compose down`; `GUSTO_DATA` stays. The project is named `gusto`. Set `COMPOSE_PROJECT_NAME` to run a second install on the same machine. For rootful podman, run `systemctl enable --now podman.socket` and set `GUSTO_RUNTIME_SOCKET=/run/podman/podman.sock`. That has not been run, and rootless podman has not been tried.
