# The public cloud

`gusto cloud` is the hosted server for accounts and routing. A person signs up, creates an organization, and the organization gets its own `gusto serve`.

## How organizations are kept apart

Each organization runs its own `gusto serve` process with its own data directory, signing key, credentials, spend ledger, change log and task queue. gusto cloud holds accounts and routes requests by host name (browser, gusto cloud, the organization's server). It runs no tasks.

A paying organization's tasks run in OpenShell sandboxes on gateway hosts. A sandbox belongs to one task, is deleted when the task ends, and is never reused for another organization. Each sandbox is a microVM with its own kernel. A demo's tasks run in Docker containers on the cell, one for each turn, which share the cell's kernel ([Demos](#demos)).

## Where it runs

CI and the running service make everything. Nothing is made or changed from a person's machine.

| Layer | Made by | When |
| --- | --- | --- |
| Base stack: network, control-plane host, host image and template, storage, keys, DNS, roles | Terraform and Ansible in GitHub Actions, as an IAM role through OIDC, with state in an encrypted S3 bucket | when the platform changes, from a reviewed pull request |
| An organization: record, directory, server, route, secrets path | the orchestrator, on the control-plane host | at sign-up |
| Capacity: more or fewer hosts | the orchestrator, through the AWS API | as load changes |
| Dedicated or customer-owned infrastructure | a provisioner worker running the same Terraform modules, with state for each organization | when an organization asks |

A host is one machine with KVM. It runs an OpenShell gateway and the `gusto serve` processes of its organizations. The account store says which host an organization is on. A new organization on the shared pool needs no new AWS resources. On an `m8i.2xlarge`, 12 sandboxes ran together, and CPU-bound work ran at full speed up to four ([OPENSHELL.md](OPENSHELL.md)). Host capacity is a setting.

A controller in gusto cloud moves an organization through `provisioning`, `ready`, `suspended` and `deleting`. Each step is safe to repeat, and a failed step is retried and shown to the operator. Only the local host, this machine, is built. The other hosts and the AWS calls are not. The orchestrator's IAM role can scale the hosts' group, read the secrets under its own path and snapshot their disks. It cannot change the base stack. A person with admin access runs the first apply of the base stack. CI runs every later change, `down` and `reset`.

## Settings

| Setting | Meaning |
| --- | --- |
| `GUSTO_CLOUD_ROOT` | where organizations' files are kept, and `cloud.db` when there is no database address. `cloud` by default |
| `GUSTO_DATABASE_URL` | where the accounts are kept. Unset, SQLite in `cloud.db` under the root. A `postgres://` address (its `search_path` names the schema, made when missing) lets several front doors share them ([CLOUD-DATABASE.md](CLOUD-DATABASE.md)) |
| `GUSTO_CLOUD_INDEX_DIR` | the directory for organizations' indexes of model calls and tasks when an organization's own `GUSTO_DATABASE_URL` is not Postgres: `<dir>/<slug>/index.db` on the local disk of the host the server runs on. `index` under the root by default. Set it to a local path when the root is on a disk that hosts share |
| `GUSTO_CLOUD_ADDR` | where gusto cloud listens, `127.0.0.1:8050` by default |
| `GUSTO_CLOUD_URL` | the address links in mail begin with |
| `GUSTO_CLOUD_DOMAIN` | the domain organizations are served under, as `<slug>.<domain>` |
| `GUSTO_CLOUD_MAIL` | `outbox`, `smtp://user:password@host:587?from=name@example.com`, or `cloudflare://<account id>?from=name@example.com` |
| `GUSTO_CLOUD_MAIL_TOKEN` | the Cloudflare API token that may send mail, for `cloudflare://`. Keep it where a secret is kept |
| `GUSTO_CLOUD_LEASE_FILE` | the file that says when a demo host removes itself. A person signed in at `/lease` extends it, and every page warns in its last five minutes. Without it there is no lease |
| `GUSTO_CLOUD_LEASE_STEP_MINUTES` | how much each extension adds, 30 by default |
| `GUSTO_CLOUD_MEMBERS`, `GUSTO_CLOUD_COMPUTERS` | people and workspaces a verified organization may have, 4 and 4 by default: an admin and three users |
| `GUSTO_CLOUD_ORG_ENV` | a file of `KEY=VALUE` lines that every organization's server starts with: the operator's credentials directory, models and sandbox. Nothing else of gusto cloud's environment goes to them |
| `GUSTO_CLOUD_PORT_FROM` | the first port an organization's server gets, two each, 9100 by default |
| `GUSTO_CLOUD_START_WAIT_SECONDS` | how long a server has to answer once started, 120 by default |
| `GUSTO_CLOUD_SERVERS` | `off` runs gusto cloud with accounts and no organization servers |
| `GUSTO_CLOUD_FREE_MAIL` | `allow` lets free mail providers sign up, for a demonstration |
| `GUSTO_CLOUD_STRIPE_KEY`, `GUSTO_CLOUD_STRIPE_PRICE`, `GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET` | bill organizations by the seat ([Billing](#billing)): the secret API key, the monthly price of one seat, and the webhook's signing secret. All three or none. Keep the key and the secret where secrets are kept |
| `GUSTO_CLOUD_SEAT_USD` | the seat's price the organization page shows, 25 by default |
| `GUSTO_CLOUD_PAID_JOBS_PER_SEAT`, `GUSTO_CLOUD_PAID_MEMBERS` | a paying organization's tasks a month for each seat, 200, and the people and workspaces it may have, 50 |

## Accounts and sign-up

gusto cloud keeps people, organizations, invitations, single-use links, demos, leads, demo limits, the pause, the signing key for cookies, the abuse counts and each organization's runtime in a database ([CLOUD-DATABASE.md](CLOUD-DATABASE.md)): SQLite in `cloud.db` under `GUSTO_CLOUD_ROOT`, or Postgres. Under the root it keeps `orgs/<slug>/` for each organization and `outbox/` (mail, when mail goes to files). Each organization's directory `orgs/<slug>/data/` (0700) is its `gusto` data directory. gusto cloud also writes `quota.json`, `members.json` and `inbox/` there, and the organization's server writes `settings.json`. An earlier release kept the accounts as files (`people/`, `orgs/<slug>/org.json`, `tokens/`, `demos/` and a few more); the first start on a database with no import record loads them and leaves them as they were.

1. A person gives a work email. The address is refused when it is disposable, or at a free mail provider unless the operator allows it.
2. The organization is named from the company's domain, found with the public suffix list: `ada@acme-research.com` makes "Acme Research", with the address `acme-research-com`. A free mail address names no company, so the person is asked for a name.
3. The person, the organization and its directory are made, the person is signed in, and a verification link is mailed.
4. Until the link is followed, the organization may run a small trial. Following it lifts the limit. Until then, the side panel of the organization's pages reminds the person and mails the link again. It asks the front door's `/account.json` and posts to `/account/confirm`, which answer an organization's own address and no other.

A company can have more than one organization. The first at a domain gets the domain's address, and each later one is named by whoever makes it. A confirmed link proves one mailbox, not the domain, so nobody joins an organization by having an address at it. People join by invitation. Domain proof by DNS record is planned (getlatentic/planning#396).

| Abuse limit | Default |
| --- | --- |
| Sign-up attempts from one address, refused ones too | 10 an hour |
| Mail to one address | 3 an hour |
| Sign-ups in all | 200 an hour |
| Organizations for one company domain whose admin has not confirmed their address | 3 |
| Invitations for one organization | 20 an hour |

The counts are kept in the database, so front doors that share it share the limits, and a restart does not forget them.

## Sign-in to an organization

1. The person signs in at gusto cloud by a mailed link. A sign-in link is always answered the same way, so the form cannot show who is a customer.
2. gusto cloud sends the person straight into their organization through `/open`. While the organization's server starts, `/open` shows what it is doing and looks again every few seconds. The organization's details, people and billing are at `/org`.
3. Open makes a token, good for a minute and one use, signed with a key that only gusto cloud and that organization's server hold. It sends the browser to `<slug>.<domain>/auth/entry`.
4. The server checks the token and signs the person in. It has no password page, and `/login` there sends people back to gusto cloud.
5. The server trusts the token for an hour. Then the person goes back through gusto cloud, which asks whether they still belong.
6. Sign out in the organization's server ends its session. It then sends the browser to gusto cloud's `/signout` with a token for that purpose only, signed with the same key and good for a minute, and gusto cloud signs the person out too. A link to `/signout` without that token signs nobody out.

gusto cloud's pages allow a form to end at any address under its domain (`form-action`), because sign-in, sign-up and joining redirect into an organization's server.

One address is one sign-in, and a person can be in several organizations, with a role in each. A person in more than one chooses at `/orgs` after signing in, and the choice is remembered. Each row there goes into the organization, and its Settings link goes to `/org`. `/org?org=<slug>` and `/open?org=<slug>` choose one of theirs; a name of an organization they are not in chooses nothing. Each organization's server sends people back with its own name. The mark at the top of its rail opens a switcher: it lists the person's organizations from the front door's `/orgs.json`, which answers an organization's own address and no other, and opens each through `/open`. A server knows only its own members, and its own name from `GUSTO_ORG_NAME`.

## People

An admin with a confirmed address invites people from the organization's page. An invitation counts against `GUSTO_CLOUD_MEMBERS`. The link opens a page that asks the person to press a button, so a mail program that opens every link does not answer it. The person joins as a runner. An address that is already in another organization is invited the same way and keeps that organization.

An admin can cancel an invitation, remove a person (not themselves or the last admin), and make a person an admin or a runner. A removed person keeps their other organizations. A person removed from their only organization loses their sign-in to gusto cloud at once. Their organization server stops trusting them, and a changed role takes effect, within the hour.

gusto cloud tells the organization's server about people through two files in its data directory. `members.json` lists people, roles, last sign-in and unanswered invitations. If the server cannot read it, the People page says so. `inbox/` holds one event per invitation, role change and removal. The server is the only writer of the change record. It takes the events in order and removes each file. A file of another kind is set aside as `.rejected`.

## Billing

With the Stripe settings set, an organization pays $25 a month for each person in it. A demo is never billed.

- An admin presses Subscribe on the organization page and pays on Stripe's Checkout page, for a seat for each member. The organization's slug goes on the subscription.
- Stripe sends `customer.subscription.*` events to `/stripe/webhook`. An event is believed only with a `v1` signature from the webhook secret, made in the last five minutes. An event older than the one kept changes nothing, so events can arrive in any order or twice.
- A subscription that is `active`, `trialing` or `past_due` gives the paid quota: `GUSTO_CLOUD_PAID_JOBS_PER_SEAT` tasks a month for each seat. `past_due` is the grace while Stripe tries the card again. One that is `canceled`, `unpaid`, `paused` or `incomplete_expired` starts no new task and invites nobody. It keeps its people, workspaces and work, and an admin renews it by subscribing again. With no subscription an organization stays on the trial and verified limits.
- The seats billed follow the members. Joining or being removed wakes the seat sync, which also runs every minute, and Stripe prorates the change. A change Stripe refuses is tried again on the next run.
- An admin opens Stripe's portal from "Card, invoices and plan" to change the card, read invoices or end the subscription.
- A second subscription for an organization that already pays, from two tabs, is not kept and is logged, for the operator to refund.

In Stripe, make a product with a monthly per-unit price and give its id as `GUSTO_CLOUD_STRIPE_PRICE`. Add a webhook endpoint at `<GUSTO_CLOUD_URL>/stripe/webhook` that sends the `customer.subscription.created`, `.updated`, `.deleted`, `.paused` and `.resumed` events, and turn on the customer portal.

## Workspaces

A person has one workspace in an organization, made the first time they run a task. Its record is in the organization's directory (`internal/computer`) with these fields: `user`, `org`, `sandbox`, `host` (the host machine), `state` (`creating`, `running`, `stopped`, `failed` or `deleted`), `created` and `lastActive`. The `sandbox` name is `c-`, eight characters of a digest of the organization, and eight of the organization and the person.

- A task works in `/sandbox/jobs/<job>/workspace`, removed when the task ends. A turn in a conversation on an instance works in `/sandbox/jobs/instance-<id>/workspace`, kept between turns.
- A task from a script or token has no person and runs in a sandbox of its own that goes with the task.
- A person past the organization's number of workspaces is refused in the task's record, and nothing is made.
- A workspace idle for half an hour with no task running is stopped, which keeps its files. Set `GUSTO_COMPUTER_IDLE_MINUTES` to change it. The next task starts it. Removing a person marks their workspace, and the organization's server removes it.
- A person shares a web app their agent built from the agent's page. Members open it at the organization's address ([OPENSHELL.md](OPENSHELL.md)). `GUSTO_APPS` is where the server keeps its record.
- A turn that leaves an `index.html` in the agent's folder gets a Preview button. It starts a static file server, which needs only node, on the first free port from 8080.
- `gusto cloud workspaces` lists every workspace on the host with its organization and person. It also reports a sandbox with no record, or a record with no sandbox.

## The admin console

An organization's server has a console for its admins at `/admin`. A person who runs work gets 403 on every console page and form. A section the server has nothing for is not listed and answers 404. The old addresses `/changes`, `/changes.jsonl`, `/tokens` and `/retention` redirect into the console.

| Section | Address | Holds |
| --- | --- | --- |
| Overview | `/admin` | this month's spending (UTC), limits and use, tasks running, waiting and failed in the last seven days, workspaces, shared apps, the budget |
| Spending | `/admin/spending` | the spend report for the whole organization |
| Activity | `/admin/activity` | the change record, downloadable at `/admin/activity.jsonl` |
| Tokens | `/admin/tokens` | API tokens |
| Retention | `/admin/retention` | the disk, the policy, sweeps and holds ([RETENTION.md](RETENTION.md)) |
| Skill sets | `/admin/skill-sets` | skill sets and their skills |
| People | `/admin/people` | each person's role, last sign-in and spend this month, and unanswered invitations. Inviting, removing and changing a role happen at gusto cloud |
| Models | `/admin/models` | providers, models and limits per person |

On the Overview, an admin sets one alert: a monthly budget in dollars. It is kept in `settings.json` in the organization's data directory, and `GUSTO_ORG_SETTINGS` names it. When spending passes the budget, a banner shows the excess, "at least" when some spending has no price. The budget stops no task (a team's cap does) and sends no mail. An unreadable quota is shown as unreadable, not as no limit.

An admin also chooses what a new chat runs on, under New chats. A person's last choice comes first. With no admin choice, a new chat starts on `GUSTO_CHAT_DEFAULT` (`agent=model`), then on the first agent and model that can run.

## Free credit

Each organization may spend a credit on the models gusto provides, the operator's keys: 5 dollars for a sign-up (`StartingCredit`), 0.50 for a demo. It is used once and does not come back each month.

- gusto cloud writes the credit into the organization's `quota.json` as `creditUsd`, and its server reads it as it changes. Every start of gusto cloud writes each organization's quota again, so a change of limits reaches organizations made before it.
- The server counts every call on the operator's keys against it, in any month, and writes what is used to `credit-used.json` beside the quota. That file only grows, so deleting task records gives nothing back.
- Once it is used, the proxy refuses the next call on the operator's keys, and refuses a model with no price. Calls in flight can pass it by a few cents.
- A call on a key the organization added at `/admin/models` is never counted against it or stopped by it. Team and task caps still count every call.
- The organization page says what is left. The admin overview shows it as a limit, and, once the organization has used a key of its own, its spending by whose key paid. A turn the credit stopped says so, and links an admin to `/admin/models`.

## Models

Admins choose the models their people may use at `/admin/models`. A provider is an address, how it takes its key, and perhaps the agents it works with. A model belongs to a provider and may have a price in dollars for a million tokens of input, output and cached input. A team's cap cannot count a model with no price.

- Each change is a version of the registry. It is written to the change record without the key and applies to the next model call. A running task keeps the credential it began with. `out/proxy.jsonl` records the version of each call.
- A provider or model an admin adds is off until an admin turns it on. The registry keeps its last twenty earlier states, and "Go back to it" restores one as a new change.
- A key is typed once, kept in a file only the server can read, and never shown again. Retiring a provider forgets its key.
- `GUSTO_REGISTRY_KEYS` allows naming a key as `file:<path>`, `ssm:<name>` or `secretsmanager:<id>`. A deployment that hosts other people's organizations leaves these out, because they let an admin name anything the machine can read.
- A provider must be at an https address that resolves to a public address. The check runs when it is added and on each proxy connection. A call that would reach this machine, a private network or a cloud metadata service is refused. `GUSTO_REGISTRY_PRIVATE=allow` lifts this for a model inside your network. Behind gusto cloud the environment fixes both settings.
- An admin can limit a person to some models. API tokens belong to nobody, so they are not limited.

## Demos

With `GUSTO_CLOUD_DEMO_MINUTES` set, `/demo` lets a visitor start a demo with a name, a work email and a company. They get a signed-in organization on a small quota (one workspace, one person). Their time starts when the server first answers, with one extension and a link to book a call (`GUSTO_CLOUD_BOOK_URL`). When time is up, gusto cloud asks the server to remove its workspaces, waits up to ninety seconds, stops the server and deletes the organization, its memberships, its data, and the accounts of people who were in no other organization. A demo not ready within `GUSTO_CLOUD_DEMO_PATIENCE_MINUTES` is deleted the same way. Only an organization made as a demo is deleted this way. With `GUSTO_CLOUD_DEMO_STAFF` set, only the addresses and domains it names may start a demo. Everyone else is offered sign-up, or a demo booked with the team.

- Set `GUSTO_CLOUD_TURNSTILE_SITE` and `GUSTO_CLOUD_TURNSTILE_SECRET` to add a Cloudflare Turnstile bot check. It fails closed.
- At most `GUSTO_CLOUD_DEMO_MAX` demos run at once, and `GUSTO_CLOUD_DEMO_PER_DAY` (40) start in a day.
- Pause new demos on the operator's page. Resume there. Running demos continue.
- Each visitor is kept as a lead in the database, which stays after the demo ends.
- A visitor's agent runs each turn in a Docker container on the cell (`demo_sandbox: container` in `deploy/aws/openshell/ansible/roles/demo/defaults/main.yml`). The container has no network (`--network=none`): the proxy's socket, mounted in, is its only way out. It has a cap on CPU, memory and processes, and only five capabilities. Containers share the cell's kernel, so a flaw in the kernel could let one visitor's agent reach another's on the same cell. An organization that pays runs its agents in OpenShell microVMs (`demo_sandbox: openshell`).
- Each container is labelled `com.getlatentic.gusto.org`, `.person`, `.instance` and `.job`. The person is an opaque id, the same as their computer's, never their address. `docker ps --filter label=com.getlatentic.gusto.org=<slug>` lists one organization's running agents. When an organization's server stops, or starts again, the cell removes its containers that are left.
- A demo has a credit of 0.50 dollars on the models gusto provides, and a sign-up has 5 dollars ([Free credit](#free-credit)).

## The operator's page

`gusto operator` serves the page the operator manages a hosted gusto from: demo requests (with a spreadsheet download), demos running, a switch that pauses new demos, the demo limits and the hosts that are up. The limits are the most demos in a day, the most one address and one company may start in a day, and the most at once. They hold from the next demo, on every front door, and are kept in the database. `GUSTO_CLOUD_DEMO_PER_DAY`, `GUSTO_CLOUD_DEMO_PER_PERSON`, `GUSTO_CLOUD_DEMO_PER_COMPANY` and `GUSTO_CLOUD_DEMO_MAX` are the starting values.

It is the separate service `gusto-owner`, listening on `127.0.0.1:8090` of gusto cloud's host only. It refuses any other address (`GUSTO_OWNER_ADDR`), any request that names another host or comes through a proxy, and any form from another site. Run `deploy/aws/openshell/owner.sh` to open a tunnel through AWS Systems Manager to `http://localhost:8090`. It needs the Session Manager plugin: `brew install --cask session-manager-plugin`.

## Checks and costs

- Rehearsal. `deploy/aws/openshell/demo-up.sh` makes a host with a public name, a certificate and mail. `deploy/aws/openshell/e2e-cloud.sh` runs sign-up, invitations and agents through the pages with a real model and checks what the host holds.
- Isolation. Start a demo and an agent in it, then run `deploy/aws/openshell/check-isolation.sh` while the agent works. It probes from inside the agent's container or sandbox and passes only when the model proxy is open and nothing else is: the AWS metadata address, the machine's ports, gusto cloud, the shared file system, the VPC resolver and the internet. It exits 0 on a pass. Run it after a change to the sandbox policy, the OpenShell version or the machine's network. It does not test UDP other than names, or IPv6.
- Budgets. `deploy/aws/platform/budgets.tf` makes two AWS budgets that mail `ALERT_EMAIL` when `bootstrap.sh` runs with it set: `monthly_budget_usd` (60 dollars) mails at 80% and when the forecast passes 100%, and `daily_budget_usd` (10 dollars) mails when the day passes it. A demo machine left on costs about 5.30 dollars a day.
- Cold start. With no demo machine running, a visitor waited 270 seconds on 2026-10-02 with no image. `deploy/aws/openshell/bake-cell-image.sh` makes a host image: about 25 minutes, and it writes the image id to `/gusto/poc/cell-ami`. The demo bundle workflow runs it after every merge that changes the key from `bake-key.sh`, and it does nothing when an image of that key exists. A host with an older image installs everything and is slower.
- Cell kit. The front door that starts cells runs `ansible/cell-kit.yml` once a release. It writes a cell's files to `kits/<sha256>.tgz` in the demo's directory of the shared file system, and starts each cell with that sum. A cell from the matching image mounts the file system, checks the sum, unpacks the kit and starts its services, with no playbook and no download. If the kit is missing, its sum differs or the image differs, the cell runs the playbook.
- Releases on running cells. A cell keeps its own release, but the organizations' servers on it run the front door's. The front door copies its program to `releases/<sum>/gusto` under `GUSTO_CLOUD_ROOT`, on the shared file system, and cells start servers from there. Each organization's runtime records the command its server was started with. A front door that starts and finds a server running with another command stops it and starts it again as its own release. A deploy therefore restarts every organization's server once, and stops the turns running then. The newest three copies are kept, and any copy younger than a day.
- Kept cell. A cell of the live front door is an `m8id.xlarge`, which has a disk of its own. The gateway keeps each sandbox and its records there, and AWS blanks that disk whenever the host stops. The front door keeps one cell (`demo_cell_keep`): when it is idle it is stopped, not removed, and when a cell is needed it is given the current start-up script and started again. Its root disk holds only the image's files, so a stopped kept cell costs that 40 GB disk, about 3.20 dollars a month once the disk is on the baseline throughput. A new disk is read from the image's snapshot a block at a time and needs the launch template's throughput: on the baseline a start-up took 41 s, with the first read of a parameter timing out, against 8.5 s. The front door lowers the kept cell's disk to the baseline when it first stops it. On 2026-10-25 a kept cell was running 8.9 s after it was started, and a new one about 17 s after it was launched. When the kept cell is busy, a new cell is launched. A kept cell is launched to stop, not be removed, when it powers itself off at its twelfth hour of uptime or because the front door has not been heard from, so it can be started again; every other cell is removed when it powers off. The front door's scaler lists the stopped cells each time it decides: a stopped cell that is not the kept one, or is beyond `demo_cell_keep`, is removed, and the kept one's disk is lowered to the baseline if it stopped itself and so was not. A kept cell made before this is given the same behaviour when it is next started. A stopped kept cell with an older image is started like any other and runs the playbook, which is slower and correct.
- No cloud-init. The image of a cell has cloud-init turned off. `gusto-launch` runs the host's start-up script on every start, and the network is one for any interface. The image has no sshd and no SSH host keys, since cloud-init made new keys for each host and without it every host would share them; a cell is reached through Systems Manager.
