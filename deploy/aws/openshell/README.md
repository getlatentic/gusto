# OpenShell on AWS

One instance with nested virtualization, OpenShell's gateway on it with the VM
driver, and a probe that says what a sandbox may and may not do. It is separate
from the E2B POC in the directory above: its own Terraform state, and no DNS,
budget alarm or E2B services. It uses the same `haas-poc` AWS profile and the
same tag, `project=haas-poc`.

## Cost

| state | cost |
| --- | --- |
| up | about $0.21 an hour for an `m8i.xlarge`, plus a 40 GiB disk |
| down | nothing: `down.sh` removes the instance, disk, group and key |

## Run

```bash
./up.sh       # Terraform makes the instance; Ansible installs OpenShell and starts the gateway
./probe.sh    # starts two sandboxes and runs probe/inside.sh in one; output in out/probe.txt
./down.sh     # removes everything, then asks AWS whether anything tagged haas-poc is left
```

`up.sh` makes an SSH key under `.state/` and opens port 22 to this machine's
address only. `down.sh` removes the key.

## What the probe measures

`probe/expected.txt` is the output of the first run, 2026-09-29, on an
`m8i.xlarge` in us-east-1, with the start times written as `N.N`. Those were
8.2 s for the first sandbox, which pulls a 512 MB image, and 6.5 s for the
second.

| Line | Says |
| --- | --- |
| host kernel, `/dev/kvm` | nested virtualization reached the instance |
| guest kernel | the sandbox is a VM with its own kernel |
| `dns example.com` | a name resolves to the sandbox's policy proxy, not to the internet |
| `tcp ...: refused` | direct connections are refused, the metadata address and a VPC address included |
| `write ...` | `/etc` and `/root` are not writable and `/tmp` is |
| `read /etc/shadow: allowed` | the default policy allows reads that haas's policy should narrow |

## The end-to-end runs

`up.sh` and `probe.sh` show what a sandbox is. Two more scripts run haas on the
instance with `HAAS_SANDBOX=openshell`:

```bash
./e2e.sh           # one job: a harness in a sandbox asks the model through haas's proxy, and tries a host nobody allowed
./e2e-compare.sh   # one comparison from the pages' form, in sandboxes made from a Node root file system
```

`e2e-compare.sh` makes the root file system from `node:24-slim` for x86-64 with
docker or podman on this machine, and copies it and a Linux build of haas to the
instance. Both keep their output in `out/`. The first run's result:

```
verdict: fixer was the only one to pass the check.
fixer Fake | Tests pass | 9 s | 1 file +1 -1
idler Fake | Tests fail | 9 s
```

Each record was signed, and its `sandbox-events.log` and `sandbox-policy.json`
were among the files the manifest hashes.

## A real harness on a real model

`e2e-real.sh` runs opencode, then a comparison of opencode and pi, from the
runner image. Without a model it uses one that only answers "pong". With a
model it reads the key on the instance, from Parameter Store, with the
instance's own role. That role may read `/haas/poc/*` and nothing else.

```bash
./up.sh
./put-key.sh                                   # once: hidden prompt, key goes to AWS on standard input
HAAS_MODEL_UPSTREAM=https://bedrock-mantle.us-east-1.api.aws HAAS_MODEL_NAME=bedrock \
  HAAS_MODEL_ID=qwen.qwen3-vl-235b-a22b-instruct ./e2e-real.sh
./down.sh
```

The standard tier of Parameter Store and the AWS-managed key cost nothing.
The parameter outlives `./down.sh`. Delete it with
`aws ssm delete-parameter --name /haas/poc/model-key`.

## The box's matrix

`e2e-matrix.sh` runs `docs/matrix/box.json` on this backend: thirteen pairs across
six harnesses, each started in a sandbox made from an image that carries them all.
`image.sh` builds that image on the instance. The instance reads both keys from
Parameter Store, so store DeepSeek's too:

```bash
python3 -c 'import json,os;print(json.load(open(os.path.expanduser("~/.viva/auth.json")))["deepseek"]["apiKey"])' \
  | HAAS_MODEL_PARAMETER=/haas/poc/deepseek-key ./put-key.sh
./e2e-matrix.sh                       # about 15 minutes after the image is built
AGAIN=all ./e2e-matrix.sh             # or AGAIN=codex@deepseek/deepseek-flash,... for chosen pairs
```

## Two harnesses in one sandbox, a shared app, and many sandboxes

```bash
./e2e-shared.sh        # claude, then codex, on one project in one sandbox; the app they wrote, shared through the gateway
./demo-up.sh           # a host that serves the public cloud at https://$DEMO_NAME (see below)
./demo-down.sh         # its DNS records, then the host
./e2e-cloud.sh         # the public cloud as people use it: sign-up, three invited people, a computer each, a second organisation
./e2e-concurrency.sh   # 1, 2, 4, 8 and 12 sandboxes at once, in waves of four (SIZES and WAVE change them)
```

Both need `./up.sh` and the two keys, as `e2e-matrix.sh` does. They use the same image,
which `image.sh` builds. Bring the instance up with `TF_VAR_instance_type=m8i.2xlarge` for
the sizes in `docs/OPENSHELL.md`. Two things cost time: `openshell sandbox exec` reads
standard input, so a script fed to `ssh bash -s` loses its later lines unless each
`exec` has `</dev/null`, and `pkill -f` matches the shell that runs it unless the
pattern has a bracket in it.

## Files

```
versions.tf, variables.tf, main.tf, outputs.tf   the instance, its group and its key
up.sh, probe.sh, down.sh, env.sh                 the three commands, and what they share
ansible/                                         the role: pinned, checksummed binaries, certificates, the gateway's service
probe/inside.sh                                  runs inside a sandbox
probe/on-host.sh                                 runs on the instance and starts the sandboxes
probe/expected.txt                               what the first run printed
e2e.sh, e2e-compare.sh, e2e-real.sh, e2e/         haas on the instance, with jobs in sandboxes
e2e-matrix.sh, image.sh                          the box's matrix, and the image it runs from
put-key.sh, put-cloudflare.sh                    store a model's key, and the two Cloudflare tokens, in Parameter Store
demo-up.sh, demo-down.sh, demo-store.sh          a host on a public name, its removal, and the bucket that outlives it
demo.env.example, demo/, ansible/demo.yml        its settings, the program that sets DNS, the tests of the boot script and the lease reaper, the role
demo-sweep.sh                                    terminates a demo host that has been up longer than any may be
e2e-cloud.sh, e2e/cloud/                         the front door driven through its pages, on a real model
e2e-shared.sh, e2e-concurrency.sh, e2e/shared/   two harnesses in one sandbox, a shared app, many sandboxes
harnesses.sh, e2e/credentials.sh                 the harness list and the proxy's credentials, shared by the runs
```

The binaries are OpenShell 0.1.2. Their checksums are pinned in
`ansible/roles/openshell/defaults/main.yml`, from the release's own checksum
files, so a changed asset fails the install.

## A demo host on a public name

`demo-up.sh` is the way to try a change from a laptop. The way to make a host without one is the
workflows described in `../platform/README.md`.


`demo-up.sh` makes a host that serves the public cloud at `https://<name>` for the front
door and `https://<slug>.<name>` for each organisation, with a certificate and mail that
arrive. It costs nothing beyond the host: DNS records are not proxied through Cloudflare,
Let's Encrypt is free, and the store bucket costs cents.

| Piece | How |
| --- | --- |
| Name | An A record for `<name>` and one for `*.<name>`, set by `haas-dns` when the host starts, 60 s TTL. |
| Certificate | Let's Encrypt, for `<name>` and `*.<name>`, by a DNS challenge through Cloudflare. It is kept in the store bucket and reused until it has under 30 days left. |
| Web server | nginx on the host passes to the front door on `127.0.0.1:8050`, and answers no name that is not this one. |
| Mail | Cloudflare Email Sending, `HAAS_CLOUD_MAIL=cloudflare://<account id>?from=...`. |
| Image | Built on the first run, ten minutes, and kept in the store bucket. |

Once, from your machine:

```bash
./put-key.sh            # the model key
HAAS_MODEL_PARAMETER=/haas/poc/deepseek-key ./put-key.sh   # and the second
./put-cloudflare.sh     # the DNS token, then the mail token
cp demo.env.example demo.env   # and fill it in
```

Each demo:

```bash
./demo-up.sh            # about 15 minutes the first time and fewer after
./demo-down.sh          # removes the DNS records and the host, and says if anything is left
```

A demo host removes itself. When it starts it writes a lease, `DEMO_LEASE_MINUTES` long
(thirty by default), and a timer on the host powers it off when the lease ends. The instance is
launched to terminate on shutdown, so the power-off removes it and its disk, and the host needs no
AWS credential to do it. A person signed in at `https://<name>/lease` can add thirty minutes at a time,
and every page shows a banner for the last five minutes with a link to that page. No extension passes
`DEMO_MAX_HOURS` (four by default), and a host that has been up that long goes whatever its lease says.
`demo-sweep.sh` is the backstop from somewhere that is not the host: it terminates anything tagged
`haas-poc` that has been up more than `MAX_HOURS` (six by default).

The DNS token is limited to the zone's DNS records. The mail token can send mail and
nothing else. Both stay in Parameter Store, and the host reads them with its own role.
