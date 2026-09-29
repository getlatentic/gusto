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

## Files

```
versions.tf, variables.tf, main.tf, outputs.tf   the instance, its group and its key
up.sh, probe.sh, down.sh, env.sh                 the three commands, and what they share
ansible/                                         the role: pinned, checksummed binaries, certificates, the gateway's service
probe/inside.sh                                  runs inside a sandbox
probe/on-host.sh                                 runs on the instance and starts the sandboxes
probe/expected.txt                               what the first run printed
```

The binaries are OpenShell 0.1.2. Their checksums are pinned in
`ansible/roles/openshell/defaults/main.yml`, from the release's own checksum
files, so a changed asset fails the install.
