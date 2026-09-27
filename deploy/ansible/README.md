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

Once [autodeploy](#autodeploy) is on, a merge to main that passes its tests
deploys itself, and a hand run is for changing the vault or the box.

## What it does, in order

| role | what it adds | what it never touches |
| --- | --- | --- |
| common | `git`, `acl`, a `haas` user with a home at `/opt/haas`, the checkout | any existing user |
| autodeploy | a `haas-deploy` user whose one key runs one command, and the box's own deploy under `/opt/haas-deploy` (off until `autodeploy_enabled`) | root's keys, sshd's configuration |
| toolchain | a pinned Go under `/opt/haas/go`, and a candidate binary at `bin/haas.next` | the box's shared `/usr/local/go`, and the live binary |
| sandbox | `podman`, `uidmap`, a subuid range, a candidate image `haas:candidate` carrying `runner_harnesses` | iptables, any daemon (podman runs none), and the live image |
| store | the `haas` database, a `haas@localhost` user granted on `haas.*`, Redis db4 | every other database, user and index |
| smoke | one real job on the candidates, and one refused | the live binary and image |
| release | the candidates become live; what they replace is kept as `bin/haas.previous` and `haas:previous` | anything, when smoke failed: the play has stopped before it |
| serve | a systemd unit on `127.0.0.1:8040`, restarted and checked on `/healthz` | ports other apps bind |
| nginx | one name-based vhost for `haas.getlatentic.com`, and its certificate | the default site and every other vhost |

`serve_enabled` is **false** until the binary has an HTTP server. Until then the
playbook gives the box a working runner, which is worth having on its own: a job
directory on the box runs with `/opt/haas/bin/haas run <dir>`.

## How a deploy replaces what runs

A deploy never builds over the version that is running. The binary is built to
`/opt/haas/bin/haas.next` and the image is tagged `haas:candidate`. The smoke
job runs against those two, and only when it passes does the release role make
them live. The binary is renamed over `bin/haas`, and the `haas:dev` tag moves
to the new image. What they replace is kept first as `bin/haas.previous` and
`haas:previous`, but only when the candidate differs, so running one commit
twice never loses the version a rollback goes back to.

A smoke job that fails ends the play before the release role. The live binary,
the live image and the running process are the ones from before the deploy, and
the refused candidates stay on disk to look at.

Once the release role has run, serve restarts the service and waits for
`/healthz` to answer on `127.0.0.1:8040`. When it does not, the play puts back
what this deploy replaced, restarts on it, and still fails, so a rollback never
reads as a green deploy. `journalctl -u haas` has the reason.

## Rolling back by hand

```bash
ansible-playbook rollback.yml
```

It makes `bin/haas.previous` and `haas:previous` live again, restarts the
service and checks it answers. Running it twice lands in the same place. It
goes back one version, not to any version: there is one previous copy.

It does not move main, and the next merge deploys main again. Revert what broke
on main, or [turn autodeploy off](#turning-autodeploy-off) first.

Without ansible, on the box:

```bash
cp -p /opt/haas/bin/haas.previous /opt/haas/bin/haas.restoring
mv -f /opt/haas/bin/haas.restoring /opt/haas/bin/haas
sudo -u haas -H podman tag haas:previous haas:dev
systemctl restart haas
```

## Proving a failing smoke job keeps the previous version

`smoke_fixture` names the file behind the smoke test's fixture harness.
`broken.json` changes nothing, so the job's gate fails the way a broken build's
would. On the box, write down what runs:

```bash
sha256sum /opt/haas/bin/haas
sudo -u haas -H podman image inspect --format '{{.Id}}' haas:dev
systemctl show haas -p MainPID
```

Then deploy with the broken fixture, from the laptop:

```bash
ansible-playbook site.yml -e @greencloud.vault.yml \
  --vault-password-file ~/.config/lpcg/vault_pass.txt -e smoke_fixture=broken.json
```

It has to fail at "The job runs in a container and passes its gate", because
the adder still subtracts, and no task of the release role may appear. The three readings on the box have to be unchanged, and
`/opt/haas/bin/haas.next` and `haas:candidate` are there as the refused
candidates. A run without the override then promotes them. The difference is
easiest to see right after a merge, when the candidate is built from a newer
commit than the one running.

## Autodeploy

### What it is

A merge to main runs `.github/workflows/deploy.yml`. Its tests (`gofmt`,
`go vet`, `go test -race`) gate the rest. It then connects to the box as
`haas-deploy` with one ssh key and sends one thing, the commit that passed.
That key's `authorized_keys` line forces a command, so whatever the client asks
for, sshd runs `sudo /usr/local/sbin/haas-deploy <what it asked>`. That script
refuses anything but a full commit id and starts `haas-deploy@<commit>.service`.
The unit checks the commit is on main and is not older than the one deployed,
checks it out in root's own checkout under `/opt/haas-deploy/src`, and runs this
playbook against the box itself with the vault the box holds. The output
streams back into the workflow's log.

The deploy runs in a unit rather than in the ssh session, so a dropped
connection or a cancelled workflow stops the watching and never a deploy
halfway through. One deploy runs at a time: the workflow's concurrency group
queues merges, and the box holds a lock while it deploys.

### What gains access to the box

| option | what gains standing access |
| --- | --- |
| Actions runs the playbook over ssh | GitHub holds a root key and the vault password: whoever can run a workflow with those secrets can do anything on this box, and read every secret the vault holds |
| a self-hosted runner on the box | GitHub gains a process on the box that runs whatever a workflow says, as the runner's user, plus sudo for the deploy |
| the box pulls on a timer | nothing new outside: the box already reads the repository with its read-only deploy key. Without Actions, nothing gates a deploy on the tests |
| **Actions asks, the box deploys itself** (this) | GitHub holds a key that can ask this box to deploy a commit that is already on main, no older than the one running. It cannot choose the code, run a command, open a shell or a tunnel, or read a file |

The trade, written down:

- Whoever can read the `DEPLOY_SSH_KEY` secret can make the box deploy a newer
  commit on main sooner than a merge would. That is code somebody already
  merged, so the power it adds is timing, not content.
- The box now holds the vault and its password under `/etc/haas-deploy`,
  readable by root alone. Root could already reach what the vault's values
  unlock on this box: the model keys and the sign-in secrets are written to
  `/opt/haas/var/haas.env` and the credential files, and root administers the
  MySQL the database password opens. What is new is the password itself. `greencloud.vault.yml` shares it
  with the other apps' vaults, so re-key this app's vault with a password of
  its own before turning autodeploy on
  (`ansible-vault rekey greencloud.vault.yml`) and point
  `autodeploy_vault_password_file` at it.
- The box runs a playbook it fetched as root. That was true of every hand
  deploy too, with a laptop in the middle. The checkout it runs from belongs to
  root, not to the `haas` user, so the code the server runs cannot rewrite the
  playbook that deploys it.

A box that pulls on a timer is what a sovereign customer wants, and it is one
step from this: the same unit, started by a timer on the head of main instead
of by a key.

### Secrets

Each is created under the repository's **Settings → Environments →
greencloud → Environment secrets**. Give the environment a deployment branch
rule of `main` alone, so a workflow run from another branch cannot read them.
Repository secrets (**Settings → Secrets and variables → Actions**) also work,
without that rule.

| secret | what it holds |
| --- | --- |
| `DEPLOY_SSH_KEY` | the private half of a key made for this and nothing else |
| `DEPLOY_KNOWN_HOSTS` | the box's host key, as the one line the playbook prints under "The DEPLOY_KNOWN_HOSTS secret" |
| `DEPLOY_HOST` | the address that line begins with, `91.109.118.72` |

The repository variable `AUTODEPLOY` (**Settings → Secrets and variables →
Actions → Variables**) turns the deploy job on when it is `on`. Until then a
merge runs the tests and deploys nothing.

### Turning it on, once

1. Make a key for it, and nothing else:
   `ssh-keygen -t ed25519 -N '' -C haas-deploy@github -f haas-deploy`.
2. In `host_vars/greencloud.yml`, set `autodeploy_enabled: true` and put the
   contents of `haas-deploy.pub` in `autodeploy_public_key`. The public half is
   not a secret. Commit it and merge it before step 6: the box reads host_vars
   from main when it deploys itself, and a main that says `false` removes the
   key on its next deploy.
3. Run the playbook by hand once, as at the top of this file. It creates the
   user, the sudo rule and the unit, copies the vault and its password to the
   box, and prints the `DEPLOY_KNOWN_HOSTS` line.
4. Check sshd lets the new user in: `sshd -T | grep -Ei 'allowusers|allowgroups|denyusers|denygroups'`
   on the box has to name nothing that shuts `haas-deploy` out.
5. Prove the key from the laptop. This deploys main's head, exactly as a merge
   will:
   `ssh -i haas-deploy haas-deploy@91.109.118.72 "$(git rev-parse origin/main)"`.
   Anything but a full commit id sent with that key is refused.
6. Add the three secrets, delete both halves of the key from the laptop, and
   set `AUTODEPLOY` to `on`.

### While it is on

A deploy the box runs itself leaves the vault and its password where the first
hand run put them. When the vault changes, run the playbook by hand again; that
is what copies it. A hand run does not take the box's lock, so run it when no
deploy is in progress, or ask the box for the deploy instead:
`ssh root@91.109.118.72 /usr/local/sbin/haas-deploy "$(git rev-parse origin/main)"`.

`/opt/haas-deploy/deployed` names the last commit the box deployed itself, and
`journalctl -u 'haas-deploy@*'` has every deploy's output.

### Turning autodeploy off

- **At once, without a commit:** set `AUTODEPLOY` to anything but `on`, or
  disable the `deploy` workflow in the Actions tab. Merges run the tests and
  deploy nothing.
- **Taking the access away:** set `autodeploy_enabled: false` in
  `host_vars/greencloud.yml` and deploy. The role removes the key's
  `authorized_keys` file and the sudo rule. It runs early in the play, so a
  deploy that fails further down still removes them. Then delete the secrets.
- **On the box, right now:** `rm /var/lib/haas-deploy/.ssh/authorized_keys`.

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

The box carries opencode, claude and codex, and runs jobs on opencode and codex.
Both take the proxy's per-job token instead of anybody's account. claude still
answers to a person's own signed-in account, which a shared box does not hold,
so the catalog does not offer it and says the account is what is missing rather
than the program. Carrying it costs image size and build time, and buys an image
that already has it the day its adapter takes a key the proxy holds.

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
