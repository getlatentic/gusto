# The AWS POC

One EC2 instance running E2B Embed, whose sandboxes are Firecracker microVMs,
with the POC's name in Cloudflare and a monthly budget alarm. `up.sh` makes it,
`down.sh` removes all of it.

E2B's own Terraform module makes the instance, pinned in `main.tf`: an
`m8i.2xlarge` (8 vCPU, 32 GiB) with nested virtualization, so no `.metal`
instance is needed.

## Cost

| state | cost |
| --- | --- |
| up | about $0.42 an hour for the instance, plus its 100 GiB disk and address |
| down | nothing: `down.sh` removes the instance, disk, address and network |

The budget alarm emails `git config user.email` at 80% of $100 in a month, and
when the month is forecast to pass $100.

## Before the first run

- An AWS profile `haas-poc` (`aws configure --profile haas-poc`), for the IAM
  user `haas-poc-terraform`: `PowerUserAccess` and an inline policy allowing
  IAM on `haas-poc*` roles, instance profiles and policies only.
- A Cloudflare token with DNS edit on `getlatentic.com`, in
  `~/.config/haas/cloudflare_token`, mode 600.
- Terraform 1.7.5 or newer.

## Run

```bash
./up.sh       # makes it, and waits until E2B answers
./check.sh    # E2B's smoke test on the instance: a sandbox made, a port in it reached
./bench.sh    # bench/work.py on the host and in a sandbox, and sandbox start times
./down.sh     # removes it, then lists anything still tagged haas-poc
```

The E2B API, dashboard and sandbox proxy (ports 3000 to 3002) answer only the
address `up.sh` ran from. `check.sh` runs on the instance over SSM and opens no
port.

## Files

```
versions.tf   providers: AWS, and Cloudflare with the token from the environment
variables.tf  region, instance type, hugepages, the name, the budget
main.tf       E2B's module, the DNS record, the budget
outputs.tf    the name, the address, E2B's URLs and key
env.sh        what the scripts share: the profile, the token, this address
bench/        the benchmark: work.py, and bench.mjs, run in E2B's own SDK image
```

## Measured

28 September 2026, us-east-1, three runs of `bench.sh`, seconds:

| work | host | sandbox |
| --- | --- | --- |
| 20M additions in Python | 0.481–0.484 | 0.423–0.428 |
| start 300 processes | 0.100–0.106 | 0.230–0.235 |
| 2,000 small files written and read | 0.135–0.262 | 0.086–0.089 |
| 128 MiB written with fsync | 0.167–1.076 | 0.109–0.115 |

A sandbox starts in 131–137 ms, and six at once in 159–196 ms. A sandbox's
fsync returns before the data reaches the instance's disk.

State is local (`terraform.tfstate`, not committed). Keep this directory
between `up.sh` and `down.sh`, or `down.sh` has nothing to destroy.
