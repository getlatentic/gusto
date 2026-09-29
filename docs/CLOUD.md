# The public cloud

`haas cloud` is the front door of a hosted haas. A person signs up, creates an
organisation, and the organisation gets a haas server of its own. This page
says how the parts divide, and what each stage of the build adds.

## How organisations are kept apart

Each organisation runs its own `haas serve` process. It has its own data
directory, signing key, credentials, spend ledger, change log and job queue.
The front door holds accounts and routes requests. It runs no jobs.

```
browser ──► front door (haas cloud) ──► the organisation's haas serve
             accounts, sign-in,          its own data directory and keys
             routing by host name        its jobs, in sandboxes on a gateway
```

| Question | Answer |
| --- | --- |
| Why not one server for every organisation? | `haas serve` holds one sign-in, one proxy, one ledger, one queue and one change log. Each would need an organisation on every read, and a read that misses one is a leak between organisations. A process and a directory are a boundary that no code path can forget to check. |
| What does it cost? | A process per active organisation, about the size of an idle `haas serve`. An organisation with no work can have its process stopped. |
| Where do jobs run? | In OpenShell sandboxes on a pool of gateway hosts. A sandbox belongs to one job, is deleted when the job ends, and is never reused for another organisation. Each sandbox is a microVM with its own kernel. |
| Why a shared gateway and not one per organisation? | A host with KVM is the cost. A gateway per organisation is the same isolation at about a hundred times the price, and the sandbox boundary is already per job. |

## The demo

One EC2 host runs the front door, each organisation's `haas serve` and one OpenShell
gateway.

1. An admin signs up on the public cloud and confirms their address. The
   organisation is on the trial that a verified address gets: an admin and three
   users, each with a computer.
2. The admin invites three people. An invited person signs in to the organisation.
3. Each person has one computer: a microVM on the host that keeps their files, that
   their agents run in, and that is theirs alone.
4. The admin sees every computer in their organisation, whose it is, and what state
   it is in. The operator sees every computer on the host, and whose organisation
   it belongs to.

### Computers

A computer belongs to one person and one organisation, and the organisation's server is
the only thing that makes or reaches it. Its record is kept in the organisation's own
directory, as the source of truth for what the organisation has:

| Field | Meaning |
| --- | --- |
| `user` | the person, by address |
| `org` | the organisation |
| `sandbox` | the name the gateway knows it by: `c-`, eight characters of a digest of the organisation, and eight of the organisation and the person, so what is an organisation's on the gateway is found by its digest |
| `host` | the cell it runs on |
| `state` | `creating`, `running`, `stopped`, `failed` or `deleted` |
| `created`, `lastActive` | when |

A person has one computer in an organisation. It is made when they first sign in, so a
person who was invited and never came costs nothing, and it is stopped when idle, which
keeps their files. Removing the person deletes it. The gateway's list and the
organisation's records are compared, and a sandbox with no record, or a record with no
sandbox, is reported.

## Where it runs

Nothing here is made or changed from a person's machine. What is made when
somebody signs up is made by the running service, and what is made when we change
the platform is made by CI.

| Layer | Made by | When | Runs in |
| --- | --- | --- | --- |
| The base stack: network, control-plane host, cell image and template, storage, keys, DNS, roles | Terraform and Ansible | when we change the platform, from a reviewed pull request | GitHub Actions, as an IAM role through OIDC, with state in an encrypted S3 bucket |
| An organisation: its record, directory, server, route and secrets path | the orchestrator | at sign-up, in seconds | the control-plane host |
| Capacity: more or fewer cells | the orchestrator, through the AWS API | as load changes | the control-plane host |
| Dedicated or customer-owned infrastructure | a provisioner worker running the same Terraform modules, with state for each organisation | when an organisation asks for it | the control plane, in an isolated worker |

A cell is one host with KVM. It runs an OpenShell gateway and the `haas serve`
processes of the organisations assigned to it, because a sandbox reaches its
server's proxy through the gateway machine's loopback. The account store says
which cell an organisation is on.

On an `m8i.2xlarge`, 12 sandboxes ran together, and CPU-bound work in them ran at full
speed up to four and slowed in proportion beyond that ([OPENSHELL.md](OPENSHELL.md)). A
cell's capacity is therefore a setting and not a guess: sandboxes that mostly wait for a
model can be many more than four.

A new organisation on the shared pool needs no new AWS resources: it is a
directory and a process on a cell, and a route. Only growing the pool, or an
organisation with its own, adds infrastructure.

### The orchestrator

An organisation moves through `provisioning`, `ready`, `suspended` and `deleting`.
A controller in the front door reconciles it: each step is safe to run again,
progress is written to the organisation's record, and a step that failed is retried
and shown to the operator. A crash between two steps leaves an organisation that the
next pass finishes. The controller has two ways to act on a cell, a local one that
starts processes on this machine and one that asks a `haas cell` agent on another
host, and the AWS calls sit behind an interface of their own.

The orchestrator's IAM role can scale the cells' group, read the secrets under its
own path and snapshot their disks. It cannot change the base stack. That role, and
the one CI uses, are separate so that a compromised control plane cannot rewrite
the platform.

The first apply of the base stack makes the state bucket and the OIDC role, and is
run once by a person with admin access. Every change after it runs in CI, and so do
`down` and `reset`.

## Accounts

The front door keeps its records as files under `HAAS_CLOUD_ROOT`, written by
renaming a finished file into place. A slug or an email address is claimed by
creating its file exclusively, so two sign-ups cannot take the same one.

```
root/
  secret                      signs the front door's cookies, 0600
  people/<hash of email>.json a person: email, organisation, role, when verified
  orgs/<slug>/org.json        an organisation: name, admin, when made
  orgs/<slug>/data/           the organisation's haas data directory, 0700
  orgs/<slug>/data/quota.json what the organisation may run now
  tokens/<hash>.json          a single-use link for verifying or signing in
  outbox/                     mail, when mail goes to files
```

A person belongs to one organisation. Only the hash of an email address is in a
file name, and only the hash of a link's secret is stored.

## Sign-up

1. A person gives a work email. The address is checked, and refused when it is a
   disposable one, or at a free mail provider unless the operator allows it.
2. The organisation is named from the company's domain, found with the public
   suffix list: `ada@acme-research.com` makes "Acme Research", and `mail.acme.co.uk`
   makes "Acme". Its address is the whole registrable domain, hyphenated:
   `acme-research-com` for `acme-research.com` and `acme-research-co` for
   `acme-research.co`. A domain is one company's alone, so two companies whose names
   are alike each have an address, and neither is put behind the other for arriving
   first. A free mail address names no company, so the person is asked for a name,
   and the address is made from it.
3. The person, the organisation and the organisation's directory are made, the
   person is signed in, and a verification link is mailed.
4. Until the link is followed, the organisation may run a small trial. Following
   it lifts the limit.

### A company's domain

A company can have more than one organisation: subsidiaries, divisions and teams
that keep apart, with people at the same domain. The first organisation at a domain
has the address the domain gives. Each after it is named by whoever makes it, and its
address is made from that name. A second sign-up at a domain with no name is asked for
one, and is told the company already has an organisation, in case they meant to ask
its admin for an invitation. Every organisation's page shows its domain, since two
organisations can have the same name.

A domain is written in its ASCII form, so a lookalike spelled with letters from another
alphabet is a different domain and is not given a name to show.

What is limited is organisations whose admin has not confirmed their address, three
for one domain by default. A company with all of its organisations confirmed can have
as many as it has people to make them.

A confirmed link proves one mailbox and not the domain, so it decides nothing about who
owns a domain and nobody joins an organisation by having an address at it. That waits
for a DNS record that shows the domain is the admin's (getlatentic/planning#396),
which is also what will give an organisation a mark that its domain is proven. Until
then people join by invitation.

### Abuse limits

| Limit | Default |
| --- | --- |
| Sign-up attempts from one address, refused ones too | 10 an hour |
| Mail to one address | 3 an hour |
| Sign-ups in all | 200 an hour |
| Organisations for one company domain whose admin has not confirmed their address | 3 |

A sign-in link is always answered the same way, whether or not the address has
an account, so the form cannot be used to find out who is a customer.

## Settings

| Setting | Meaning |
| --- | --- |
| `HAAS_CLOUD_ROOT` | where accounts are kept, `cloud` by default |
| `HAAS_CLOUD_ADDR` | where the front door listens, `127.0.0.1:8050` by default |
| `HAAS_CLOUD_URL` | the address links in mail begin with |
| `HAAS_CLOUD_DOMAIN` | the domain organisations are served under, as `<slug>.<domain>` |
| `HAAS_CLOUD_MAIL` | `outbox`, or `smtp://user:password@host:587?from=name@example.com` |
| `HAAS_CLOUD_MEMBERS`, `HAAS_CLOUD_COMPUTERS` | how many people and computers a verified organisation may have, 4 and 4 by default: an admin and three users |
| `HAAS_CLOUD_FREE_MAIL` | `allow` lets free mail providers sign up, for a demonstration |

## What comes next

Sign-up and the organisation record come first
(getlatentic/planning#394). Running the organisation's server and giving it
capacity follows (getlatentic/planning#395), then inviting people
(getlatentic/planning#396), then the stack on AWS from code
(getlatentic/planning#397).
