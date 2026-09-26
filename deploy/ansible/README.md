# Deploying haas

One playbook, run from a laptop, against a box that already serves other
people's sites. It is idempotent and additive: it installs what is missing,
claims one database, one user and one Redis index, adds one nginx vhost, and
leaves everything else exactly as it was.

```bash
ansible-galaxy collection install -r requirements.yml
cp inventory.example.ini inventory.ini
ansible-playbook site.yml -e @greencloud.vault.yml \
  --vault-password-file ~/.config/lpcg/vault_pass.txt
```

Code reaches the box by `git pull`, so **push the branch first**. The playbook
prints a public key on its first run; register it under the repository's Deploy
keys, read-only, and run again.

## What it does, in order

| role | what it adds | what it never touches |
| --- | --- | --- |
| common | `git`, `acl`, a `haas` user with a home at `/opt/haas`, the checkout | any existing user |
| toolchain | a pinned Go under `/opt/haas/go`, and the binary | the box's shared `/usr/local/go` |
| sandbox | `podman`, `uidmap`, a subuid range, the runner image carrying `runner_harnesses` | iptables, and any daemon: podman runs none |
| store | the `haas` database, a `haas@localhost` user granted on `haas.*`, Redis db4 | every other database, user and index |
| serve | a systemd unit on `127.0.0.1:8040` | ports other apps bind |
| nginx | one name-based vhost for `haas.getlatentic.com`, and its certificate | the default site and every other vhost |

`serve_enabled` is **false** until the binary has an HTTP server. Until then the
playbook gives the box a working runner, which is worth having on its own: a job
directory on the box runs with `/opt/haas/bin/haas run <dir>`.

## Retiring something

`retire.yml` is separate from `site.yml` on purpose: a deploy never deletes
anything, and this is run deliberately.

```bash
ansible-playbook retire.yml
```

It dumps the closer database to `/root/retired/` before anything stops, stops and
disables the three closer services, clears the failed state a unit records when
the TERM that stopped it is not in its `SuccessExitStatus`, and removes the
graspy checkout — after asserting that no service and no vhost still names
graspy. The closer checkout, vhost and database stay until
`-e remove_closer_files=true`, because stopping is reversible and deleting is
not.

Measured on 2026-09-26: closer held 212 MB of memory and 2.1 GB of disk, graspy
976 MB of disk, and in 48 hours the three closer services logged 34 requests
between them, every one a 404 from a scanner. Stopping them freed 165 MB, and
removing graspy freed 1 GB. With the services stopped and the vhost still
proxying to their port, `closer-api.tosinamuda.com` answers 502 rather than 404
until the record or the vhost goes.

## Which harnesses the image carries

`runner_harnesses` in `group_vars/all.yml` is the list, as `name=package`. The
deploy builds the image with it and reads the image's label back, and fails if
the two differ. The smoke test runs one job on a harness no image carries and
checks it is refused before it starts, with the reason in its transcript.

The box carries opencode, claude and codex, and runs jobs on opencode alone.
claude and codex answer to a person's own signed-in account, which a shared box
does not hold, so the catalog does not offer them. Carrying them buys an image
that already has them the day the proxy can stand in for that account (#85),
and a catalog that says the account is what is missing rather than the program.
It costs image size and build time, and it does not make either runnable here.

## Why a dedicated user rather than www-data

The other apps here run as `www-data`, and that is the right default. Rootless
containers need a subuid range and a writable home for the image store. Adding
those to the user that eight other services run as would change those services.
A new user changes nothing.

## Why podman rather than docker

It runs no daemon, so nothing is resident between jobs and no firewall rules are
rewritten on a box whose nginx fronts other sites. The arguments are the same as
docker's, so `HAAS_CONTAINER` is all that differs.

## Why both MySQL and SQLite

Two deployments, both real. This box has MySQL and Redis already, so haas uses
them: records in its own database, and Redis for the queue and for fanning a
job's output out to everybody watching it. A customer installing one artifact in
their own cloud gets SQLite in one file, because a second and third service to
provision is not one artifact. `store` chooses.

## What one job may take

Measured on this box on 2026-09-26, before anything was installed: 4 vCPU, 7894
MB of memory with 3100 MB available, 4095 MB of swap, 58 GB of disk with 25 GB
free, load 0.00. An `opencode` job peaked at 663 MiB in its container, so
`job_concurrency: 2` with `job_memory: 900m` is about 1.8 GB of the 3.1 GB that
was free. Raise them only against a fresh reading of `free -m`.

## Secrets

`greencloud.vault.yml` is this app's own vault, gitignored and
ansible-vault-encrypted with the box's shared vault password
(`~/.config/lpcg/vault_pass.txt`). It holds `db_password` and `haas_password`,
the shared credential that stands in for sign-in until there is an identity
provider.

## Before the first run with serve_enabled

1. Add an A record for `haas.getlatentic.com` → `91.109.118.72`, DNS-only, so
   certbot's HTTP-01 challenge reaches the box.
2. Check `ss -tlnH` for `:8040` once more; the inventory can drift.
3. Rootless podman under a system unit needs `/run/user/<uid>`, which the
   lingering session creates. Confirm it exists before trusting the service to
   survive a reboot.
