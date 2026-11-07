# The public cloud

`gusto cloud` is the hosted server for accounts and routing. A person signs up, creates an organization, and the organization gets its own `gusto serve`.

## How organizations are kept apart

Each organization runs its own `gusto serve` process with its own data directory, signing key, credentials, spend ledger, change log and task queue. gusto cloud holds accounts and routes requests by host name (browser, gusto cloud, the organization's server). It runs no tasks.

A paying organization's tasks run in OpenShell sandboxes on the nodes' gateways. A sandbox belongs to one task, is deleted when the task ends, and is never reused for another organization. Each sandbox is a microVM with its own kernel. A demo's tasks run in Docker containers on the node, one for each turn, which share the node's kernel ([Demos](#demos)).

## Where it runs

CI and the running service make everything. Nothing is made or changed from a person's machine.

| Layer | Made by | When |
| --- | --- | --- |
| Base stack: network, control-plane host, host image and template, storage, keys, DNS, roles | Terraform and Ansible in GitHub Actions, as an IAM role through OIDC, with state in an encrypted S3 bucket | when the platform changes, from a reviewed pull request |
| An organization: record, directory, server, route, secrets path | the orchestrator, on the control-plane host | at sign-up |
| Capacity: more or fewer nodes | the orchestrator, through the AWS API | as load changes |
| Dedicated or customer-owned infrastructure | a provisioner worker running the same Terraform modules, with state for each organization | when an organization asks |

A node is one machine with KVM. It runs an OpenShell gateway and the `gusto serve` processes of its organizations. The account store says which node an organization is on. A new organization on the shared pool needs no new AWS resources. On an `m8i.2xlarge`, 12 sandboxes ran together, and CPU-bound work ran at full speed up to four ([OPENSHELL.md](OPENSHELL.md)). A node's capacity is a setting.

A controller in gusto cloud moves an organization through `provisioning`, `ready`, `suspended` and `deleting`. Each step is safe to repeat, and a failed step is retried and shown to the operator. A server that stops is restarted at once the first time. One that stops again before it has stayed up for two minutes is restarted no sooner than 1, 2, 4 ... up to 300 seconds after it was last started. An organization keeps its two ports for life, unless another process holds one when its server is to start: after ten seconds of waiting for it to let go, the held port is replaced. The proxy port is the one a computer is told when it is made, so a new proxy port removes the organization's computers, to be made again; a new server port removes none. When gusto cloud stops, it waits for the servers being started and stops them with the rest, and starts none after. A server counts as started only while the process the node started is running. Only the local host, this machine, is built. The other hosts and the AWS calls are not. The orchestrator's IAM role can scale the hosts' group, read the secrets under its own path and snapshot their disks. It cannot change the base stack. A person with admin access runs the first apply of the base stack. CI runs every later change, `down` and `reset`.

## Settings

Every setting gusto cloud reads, with its default, is listed by `gusto help cloud` and in [CLI.md](CLI.md#gusto-cloud), from the one place each is declared. A value that does not read, such as a number that is not one, stops gusto cloud before it starts, with the setting's name. [CLOUD-DATABASE.md](CLOUD-DATABASE.md) says where the accounts are kept and what the leader does. [Billing](#billing) and [Demos](#demos) say what their settings do.

## Accounts and sign-up

gusto cloud keeps people, organizations, invitations, single-use links, demos, leads, demo limits, the pause, the signing key for cookies, the abuse counts and each organization's runtime in a database ([CLOUD-DATABASE.md](CLOUD-DATABASE.md)): SQLite in `cloud.db` under `GUSTO_CLOUD_ROOT`, or Postgres. Under the root it keeps `orgs/<slug>/` for each organization and `outbox/` (mail, when mail goes to files). Each organization's directory `orgs/<slug>/data/` (0700) is its `gusto` data directory. gusto cloud also writes `quota.json`, `members.json` and `inbox/` there, and the organization's server writes `settings.json`. An earlier release kept the accounts as files (`people/`, `orgs/<slug>/org.json`, `tokens/`, `demos/` and a few more); the first start on a database with no import record loads them and leaves them as they were.

1. A person gives a work email. The address is refused when it is disposable, or at a free mail provider unless the operator allows it.
2. The organization is named from the company's domain, found with the public suffix list: `ada@acme-research.com` makes "Acme Research", with the address `acme-research-com`. A free mail address names no company, so the person is asked for a name.
3. The person, the organization and its directory are made, the person is signed in, and a verification link is mailed.
4. Until the link is followed, the organization may run a small trial. Following it lifts the limit. Until then, the side panel of the organization's pages reminds the person and mails the link again. It asks gusto cloud's `/account.json` and posts to `/account/confirm`, which answer an organization's own address and no other.

A company can have more than one organization. The first at a domain gets the domain's address, and each later one is named by whoever makes it. A confirmed link proves one mailbox, not the domain, so nobody joins an organization by having an address at it. People join by invitation. Domain proof by DNS record is planned (getlatentic/planning#396).

| Abuse limit | Default |
| --- | --- |
| Sign-up attempts from one address, refused ones too | 10 an hour |
| Mail to one address | 3 an hour |
| Sign-in codes tried for one address | 5 an hour |
| Sign-ups in all | 200 an hour |
| Organizations for one company domain whose admin has not confirmed their address | 3 |
| Invitations for one organization | 20 an hour |

The counts are kept in the database, so gusto cloud servers that share it share the limits, and a restart does not forget them.

## Sign-in to an organization

1. The person signs in at gusto cloud with what they are mailed: a 6-digit code to type on the page where they asked, or a link for the device they read mail on. Using either spends both, and both last 20 minutes. A code works only for the address it was mailed to. Asking is always answered the same way, and a code for an address with no account is answered as a wrong one, so neither shows who is a customer.
2. gusto cloud sends the person to `/app`, which takes them into their organization through `/open`, or to `/orgs` to choose one. While the organization's server starts, `/open` shows what it is doing and looks again every few seconds. The organization's details, people and billing are at `/org`.
3. Open makes a token, good for a minute and one use, signed with a key that only gusto cloud and that organization's server hold. It sends the browser to `<slug>.<domain>/auth/entry`.
4. The server checks the token and signs the person in. It has no password page, and `/login` there sends people back to gusto cloud.
5. The server trusts the token for an hour. Then the person goes back through gusto cloud, which asks whether they still belong.
6. Sign out in the organization's server ends its session. It then sends the browser to gusto cloud's `/signout` with a token for that purpose only, signed with the same key and good for a minute, and gusto cloud signs the person out too. A link to `/signout` without that token signs nobody out.

gusto cloud's pages allow a form to end at any address under its domain (`form-action`), because sign-in, sign-up and joining redirect into an organization's server.

One address is one sign-in, and a person can be in several organizations, with a role in each. A person in more than one chooses at `/orgs` after signing in, and the choice is remembered. Each row there goes into the organization, and its Settings link goes to `/org`. `/org?org=<slug>` and `/open?org=<slug>` choose one of theirs; a name of an organization they are not in chooses nothing. Each organization's server sends people back with its own name. The mark at the top of its rail opens a switcher: it lists the person's organizations from gusto cloud's `/orgs.json`, which answers an organization's own address and no other, and opens each through `/open`. A server knows only its own members, and its own name from `GUSTO_ORG_NAME`.

## People

An admin with a confirmed address invites people from the organization's page. An invitation counts against `GUSTO_CLOUD_MEMBERS`. The link opens a page that asks the person to press a button, so a mail program that opens every link does not answer it. The person joins as a runner. An address that is already in another organization is invited the same way and keeps that organization.

An invitation sent from the organization's own People page (`/org/invite` takes the same `back` address as the role form) sends the person back to that page with `sent=1`; sent from gusto cloud's page it goes to `/org?sent=1`. An admin can cancel an invitation, remove a person (not themselves or the last admin), and make a person an admin or a runner. A removed person keeps their other organizations. A person removed from their only organization loses their sign-in to gusto cloud at once. Their organization server reads `members.json` at every request, at most once a second: a removed person is signed out of it, their computers' tokens stop working, and a changed role holds, from their next request.

gusto cloud tells the organization's server about people through two files in its data directory. `members.json` lists people, roles, last sign-in, unanswered invitations and the workspaces with who is in each. If the server cannot read it, the People page says so. `inbox/` holds one event per invitation, role change, removal and change to a workspace. The server is the only writer of the change record. It takes the events in order and removes each file. A file of another kind is set aside as `.rejected`.

## Workspaces

An organization holds its billing, seats, model credit, signing key, sign-in and admins. A workspace is a team's space in it, such as Engineering or Business. A workspace holds its own members, tasks and their records, conversations and rooms, agents, connections to apps, shared apps, comparisons and computers. Only its members reach them, whatever their role in the organization.

| Boundary | What keeps it |
| --- | --- |
| Between organizations | A server for each organization |
| Between workspaces | A computer for each person in each workspace, and a check of the workspace on every task, record, conversation, agent, connection and app |
| Between people | A computer for each person |
| Between tasks | A sandbox for each task on the computer |

- Every organization has the workspace `main`, named as the organization, with the admin who made it. Everything an organization kept before workspaces is in `main`, and so is everybody who was in it.
- An admin makes a workspace, renames it, and adds and removes its people at `/org`. The one who makes a workspace is in it. A workspace's id comes from its first name and does not change.
- An invitation names the workspace the person joins when the organization has more than one, and `main` otherwise.
- A person stays in at least one workspace while they are in the organization. Removing them from a workspace marks their computer in it for removal and leaves their computers in other workspaces. Removing them from the organization takes them out of every workspace and marks every computer of theirs. The mark is `computers/<id>.retire` with the id of the computer in each workspace the person was in, made whether or not a record of the computer exists, because where agents run in containers there is none; a record in a workspace they had left is marked too.
- An organization has at most 50 workspaces. A person is one seat however many workspaces they are in, and every workspace's computers count against `GUSTO_CLOUD_COMPUTERS`.
- Gusto cloud lists the workspaces and who is in each in `members.json`. The server reads them there at most once a second, as it reads roles. A server that is not behind gusto cloud has one workspace, `main`, with everybody who signs in in it.
- A request is in one workspace: an organization's API token's own, or the one the person chose in the switcher while they are still in it, or else their first. What the request makes goes there, and the pages list only what is there. Opening one thing is decided by the workspace that thing is in: a person not in it is answered 404, as for something that is not there.
- A person in one workspace sees nothing of workspaces in the organization's pages. With two or more, the switcher behind the mark at the top of the rail lists them under their organization's name, and the rail names the one they are in. `/workspace/<id>` chooses one of theirs. Gusto cloud's `/orgs.json` lists each organization's workspaces the person is in, and `/open?org=<slug>&workspace=<id>` opens one.
- A sign-in to an app is made in a workspace and used only by that workspace's tasks. A task sent to `POST /api/jobs` may name its workspace (`workspace` in `job.json`); a person's token may name any of theirs, and an organization's token only its own, which is the workspace its admin was in when they made it.
- A room's people and agents are in the room's workspace: a person in no workspace of the room's, or an agent of another workspace, is refused. An agent's name is unique in its workspace.
- The admin console is the organization's: spending, budgets, people, models and the change record count every workspace.

## Rooms

A room is in one workspace and is shared by its people and agents. Its settings are kept in the room's `instance.json`; a change is written to a copy first and held in memory only once the write succeeded, so a write that fails changes nothing.

- Guidelines are standing instructions of at most 2000 characters. Every person in the room can read them; only a room's admin (or an admin of the server) changes them. Every turn of every agent in the room starts with them, under "Standing instructions for this room". An empty text clears them.
- Mute is a person's own. A muted room adds nothing to that person's unread counts, the Unread and @me tabs or the rail badges, and nobody else sees it.
- Archive is the room's, set by an admin and not offered on a person's own conversation with an agent. An archived room leaves everybody's rail and is listed under Archived rooms; it still opens, but nothing can be said in it (a message is refused with a note, and an agent's reply does not hand work on) until an admin unarchives it.
- Each of the three is one event in the room's log, so a page that has the room open draws it again, and one entry in the change record (`room.guidelines`, with the old and new text, `room.muted` and `room.archived`). Setting a room to what it already is writes and records nothing.
- A person writes a bio of themselves (at most 600 characters) on their own page in Members; nobody writes another person's, and everybody who can open the page can read it. Every turn in a room is told what the room's people say of themselves, after who is there, up to 6000 characters in all: a person whose bio would pass that is left out of that turn. Bios are kept in `.bios.json` beside the instances, are written before they are held, and each change is in the change record (`person.bio`, with the old and new text).
- A turn that is running when a room is archived finishes and its reply is posted.

## Billing

With the Stripe settings set, an organization pays $25 a month for each person in it. A demo is never billed.

- An admin presses Subscribe on the organization page and pays on Stripe's Checkout page, for a seat for each member. The organization's slug goes on the subscription.
- Stripe sends `customer.subscription.*` events to `/stripe/webhook`. An event is believed only with a `v1` signature from the webhook secret, made in the last five minutes. An event older than the one kept changes nothing, so events can arrive in any order or twice.
- A subscription that is `active`, `trialing` or `past_due` gives the paid quota: `GUSTO_CLOUD_PAID_TASKS_PER_SEAT` tasks a month for each seat. `past_due` is the grace while Stripe tries the card again. One that is `canceled`, `unpaid`, `paused` or `incomplete_expired` starts no new task and invites nobody. It keeps its people, workspaces and work, and an admin renews it by subscribing again. With no subscription an organization stays on the trial and verified limits.
- The seats billed follow the members. Joining or being removed wakes the seat sync, which also runs every minute, and Stripe prorates the change. A change Stripe refuses is tried again on the next run.
- An admin opens Stripe's portal from "Card, invoices and plan" to change the card, read invoices or end the subscription.
- A second subscription for an organization that already pays, from two tabs, is not kept and is logged, for the operator to refund.

In Stripe, make a product with a monthly per-unit price and give its id as `GUSTO_CLOUD_STRIPE_PRICE`. Add a webhook endpoint at `<GUSTO_CLOUD_URL>/stripe/webhook` that sends the `customer.subscription.created`, `.updated`, `.deleted`, `.paused` and `.resumed` events, and turn on the customer portal.

## Computers

A person has one computer in each workspace they are in, made the first time they run a task there. Its record is in the organization's directory (`internal/computer`) with these fields: `user`, `org`, `workspace` (absent for `main`), `sandbox`, `host` (the host machine), `state` (`creating`, `running`, `stopped`, `failed` or `deleted`), `created` and `lastActive`. The `sandbox` name is `c-`, eight characters of a digest of the organization, and eight of the organization, the workspace and the person. A computer in `main` is hashed without the workspace, so a computer made before workspaces keeps its name.

- A task works in `/sandbox/jobs/<job>/workspace`, removed when the task ends. A turn in a conversation on an instance works in `/sandbox/jobs/instance-<id>/workspace`, kept between turns.
- A task from a script or token has no person and runs in a sandbox of its own that goes with the task.
- A person past the organization's number of computers is refused in the task's record, and nothing is made.
- A computer idle for half an hour with no task running is stopped, which keeps its files. Set `GUSTO_COMPUTER_IDLE_MINUTES` to change it. The next task starts it. Removing a person from a workspace marks their computer in it, removing them from the organization marks all of theirs, and the organization's server removes what is marked.
- A turn that leaves an `index.html` in the agent's folder gets a Preview button. The server serves that folder as a page, at once, at `<org>--<id>.<domain>`. On a host name of its own the page keeps what it saves in the browser, and it reaches nothing of the organization's pages.
- Where agents run on computers, a person can also share an app with a command of its own from the agent's page ([OPENSHELL.md](OPENSHELL.md)). `GUSTO_APPS` is where the server keeps its record of both.
- An app's host name is the same site as the organization's, so the server refuses any browser request that changes something unless it comes from its own pages.
- A text file shown whole has Edit, in its page and in its tab beside a chat. A save is refused when the file changed after it was opened. In a room, the room says who changed the file.
- `gusto cloud computers` lists every computer on the node with its organization, workspace and person. It also reports a sandbox with no record, or a record with no sandbox.

## The admin console

An organization's server has a console for its admins at `/admin`. A person who runs work gets 403 on every console page and form. A section the server has nothing for is not listed and answers 404. The old addresses `/changes`, `/changes.jsonl`, `/tokens` and `/retention` redirect into the console.

| Section | Address | Holds |
| --- | --- | --- |
| Overview | `/admin` | this month's spending (UTC) against the budget, split by who asked for it (the figure each person's budget is held to, with calls that named nobody as one line) and by agent type, each agent type linked to its calls on Usage. It changes nothing |
| Spending | `/admin/spending` | the spend report for the whole organization |
| Budgets | `/admin/budgets` | the organization's budget, everyone's budget, and each person's own budget and spend this month. Listed when the server keeps organization settings |
| People | `/admin/people` | each person's role, last sign-in and spend this month, and unanswered invitations on the Invited tab. A role is changed here and kept by gusto cloud; inviting and removing happen there |
| Models | `/admin/models` | the models and their prices; providers (`?tab=providers`); limits per person (`?tab=access`); a new chat's default, run tokens and earlier versions (`?tab=settings`) |
| Apps & skills | `/admin/apps` | on the Apps tab, the apps people may sign in to with their own accounts, for their agents ([GATEWAY.md](GATEWAY.md#connectors)); on the Skills tab (`?tab=skills`), skill sets and imported plugins |
| Records | `/admin/records` | whether the record is intact; the change record on the Activity tab, downloadable at `/admin/activity.jsonl`; task checks (`?tab=checks`); the disk, the policy, sweeps and holds (`?tab=keeping`, [RETENTION.md](RETENTION.md)) |

API tokens are at `/admin/tokens`, linked from the foot of People. The old addresses `/admin/activity`, `/admin/verification`, `/admin/retention`, `/admin/connectors`, `/admin/skill-sets` and `/admin/integrations` redirect to their tabs. Behind gusto cloud, the menu links to the organization's page there for its members and billing.

On Budgets, an admin sets one alert: a monthly budget in dollars for the organization. It is kept in `settings.json` in the organization's data directory, and `GUSTO_ORG_SETTINGS` names it. When spending reaches the budget, a banner says so, with the excess, "at least" when some spending has no price. The budget stops no task (a member's budget does) and sends no mail. An unreadable quota is shown as unreadable, not as no limit.

Each member can have a monthly budget too, which is enforced. Budgets sets one figure for every member, in the same form as the organization's, and a person's own from the menu on their row, which stands in place of it ($0 allows that person nothing; an empty field goes back to everyone's). Every task records who asked for it (`person` in its spec), and the proxy refuses a model call once that person's spending this month reaches their budget, saying so in the turn. A person sees their month against their budget on their Usage page. A task is held to the budget as it stands at each call, so a budget an admin lowers stops a task already running at its next call. A budget, like a room agent's allowance, refuses a call to a model with no price, since it cannot count it. Settings that cannot be read hold nobody to a member budget; the organization's caps and the credit still apply.

An admin also chooses what a new chat runs on, on the Settings tab of Models under "A new chat starts on". A person's last choice comes first. With no admin choice, a new chat starts on `GUSTO_CHAT_DEFAULT` (`agent=model`), then on the first agent and model that can run.

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

With `GUSTO_CLOUD_DEMO_MINUTES` set, `/demo` lets a visitor start a demo with a name, a work email and a company. They get a signed-in organization on a small quota (one computer, one person). Their time starts when the server first answers, with one extension and a link to book a call (`GUSTO_CLOUD_BOOK_URL`). When time is up, gusto cloud asks the server to remove its computers, waits up to ninety seconds, and ends the demo only once none is left. While one is left the demo stays marked as ending, with its server, its data and the records of its computers, and the next sweep (every tick of `RunDemos`) asks again, so a busy server removes them when it gets to them and nothing is left unnamed on the host. `gusto_demo_ends_deferred_total` counts the endings put off. A server that is down is not started again for this, so a demo whose server is down stays ending until an operator starts the server or removes its sandboxes (getlatentic/planning#831). Then gusto cloud stops the server and deletes the organization, its memberships, its data, and the accounts of people who were in no other organization. A demo not ready within `GUSTO_CLOUD_DEMO_PATIENCE_MINUTES` is deleted the same way. Only an organization made as a demo is deleted this way. With `GUSTO_CLOUD_DEMO_STAFF` set, only the addresses and domains it names may start a demo. Everyone else is offered sign-up, or a demo booked with the team.

- The landing page at `/` is shown to everybody, signed in or not. Its header offers Sign in, or Open gusto (`/app`) to a person who is signed in. A gusto cloud server with no demos has no landing page, and `/` is the way in.
- Set `GUSTO_CLOUD_TURNSTILE_SITE` and `GUSTO_CLOUD_TURNSTILE_SECRET` to add a Cloudflare Turnstile bot check. It fails closed.
- At most `GUSTO_CLOUD_DEMO_MAX` demos run at once, and `GUSTO_CLOUD_DEMO_PER_DAY` (40) start in a day.
- Pause new demos on the operator's page. Resume there. Running demos continue.
- Each visitor is kept as a lead in the database, which stays after the demo ends.
- A demo's agents run in an OpenShell microVM for each person (`demo_sandbox: openshell` in `deploy/aws/openshell/ansible/roles/demo/defaults/main.yml`). Gusto cloud gives a demo's server the lines of `GUSTO_CLOUD_DEMO_ENV` on top of `GUSTO_CLOUD_ORG_ENV`, so a demo and a sign-up on the same node run their agents in different sandboxes.
- A sign-up's agents run each turn in a Docker container on the node (`public_sandbox: container`). The container has no network (`--network=none`): the proxy's socket, mounted in, is its only way out. It has a cap on CPU, memory and processes, and only five capabilities. Containers share the node's kernel, so a flaw in the kernel could let one organization's agent reach another's on the same node.
- A person's turns in containers share a home of theirs, mounted at `/root`. It is kept under the organization's `computers/<id>/home`, so what they install and keep there is there at their next turn, as it is in a microVM. A room's turn sees only the room, and a task sent with a token is nobody's, so neither has a person's home. Each turn starts with the image's harness settings and only its own skills.
- A kept home may take at most `HomeLimit` bytes of the container backend's settings, 5 GiB when none is set. It is counted as root in a throwaway container, so what an agent made unreadable is counted too. A turn in a home over the limit does not start, and one that takes the home past it is stopped within a minute. The turn's record says so in `out/home.json`, and the error names the size and the limit. `HomeUse` gives how much of the limit a home took when it was last counted, and `ResetHome` removes a full home so its person can start again.
- A person removed from the organization has a mark `computers/<id>.retire` made for each computer of theirs. The organization's server removes the home at the next look (every minute and at start) when no turn of theirs is running in it, then clears the mark. A home with files this server may not remove goes whole to `computers/trash/`, for the job that runs as root to empty: `gusto-trash.timer` (every ten minutes) removes what is in `cloud/trash/` and in each `orgs/<slug>/data/computers/trash/`.
- Each container is labelled `com.getlatentic.gusto.org`, `.person`, `.instance` and `.job`. The person is an opaque id, the same as their computer's, never their address. `docker ps --filter label=com.getlatentic.gusto.org=<slug>` lists one organization's running agents. When an organization's server stops, or starts again, the node removes its containers that are left.
- A demo has a credit of 0.50 dollars on the models gusto provides, and a sign-up has 5 dollars ([Free credit](#free-credit)).

## The operator's page

`gusto operator` serves the page the operator manages a hosted gusto from: demo requests (with a spreadsheet download), demos running, a switch that pauses new demos, the demo limits and the hosts that are up. The limits are the most demos in a day, the most one address and one company may start in a day, and the most at once. They hold from the next demo, on every gusto cloud server, and are kept in the database. `GUSTO_CLOUD_DEMO_PER_DAY`, `GUSTO_CLOUD_DEMO_PER_PERSON`, `GUSTO_CLOUD_DEMO_PER_COMPANY` and `GUSTO_CLOUD_DEMO_MAX` are the starting values.

It is the separate service `gusto-owner`, listening on `127.0.0.1:8090` of gusto cloud's host only. It refuses any other address (`GUSTO_OPERATOR_ADDR`), any request that names another host or comes through a proxy, and any form from another site. Run `deploy/aws/openshell/operator.sh` to open a tunnel through AWS Systems Manager to `http://localhost:8090`. It needs the Session Manager plugin: `brew install --cask session-manager-plugin`.

## Metrics

The front door reports Prometheus metrics at `GET /.gusto/metrics` on its own listener (`GUSTO_CLOUD_ADDR`). The path answers only a request to `localhost`, `127.0.0.1` or `[::1]` that carries no `X-Forwarded-For` or `Forwarded` header, and is not found for any other, so a proxy or the public host name never serves it. A Prometheus, or a CloudWatch agent's Prometheus scrape, on the front door's host reads it (`deploy/alerts/prometheus.example.yml`, job `gusto-front`). Each process has a registry of its own.

| Metric | Meaning |
| --- | --- |
| `gusto_front_requests_total{class,status}`, `gusto_front_request_seconds{class}` | requests and their time. `class` is the mux pattern that matched, or `org` and `app` for what is sent on to an organization's server or a computer's app, `lease`, or `unmatched`; `status` is `2xx` to `5xx` |
| `gusto_orgs{state}` | organizations the controller holds, by waiting, provisioning, starting, ready and failed |
| `gusto_provision_seconds`, `gusto_provision_failures_total` | how long a provision took, ready or failed, and how many failed. One that waits for room is neither |
| `gusto_front_leads`, `gusto_front_epoch` | 1 when this front door runs the controller, and the lease epoch it leads with |
| `gusto_demo_ends_deferred_total` | demo endings put off because computers were still to be removed |
| `go_*`, `process_*` | the Go runtime and the process |

No label holds a person, an address, a request id or an organization. The decision on where the metrics go is Prometheus on the front door's host. The alerts are `GustoProvisionFailing` and `GustoFrontErrors` (`docs/RUNBOOK.md`). Not yet reported: the cell agent, the organization's server, the database, outbound calls, and `net/http/pprof` (getlatentic/planning#806).

## Checks and costs

- Rehearsal. `deploy/aws/openshell/demo-up.sh` makes a host with a public name, a certificate and mail. `deploy/aws/openshell/e2e-cloud.sh` runs sign-up, invitations and agents through the pages with a real model and checks what the host holds.
- Isolation. Start a demo and an agent in it, then run `deploy/aws/openshell/check-isolation.sh` while the agent works. It probes from inside the agent's container or sandbox and passes only when the model proxy is open and nothing else is: the AWS metadata address, the machine's ports, gusto cloud, the shared file system, the VPC resolver and the internet. It exits 0 on a pass. Run it after a change to the sandbox policy, the OpenShell version or the machine's network. It does not test UDP other than names, or IPv6.
- Budgets. `deploy/aws/platform/budgets.tf` makes two AWS budgets that mail `ALERT_EMAIL` when `bootstrap.sh` runs with it set: `monthly_budget_usd` (60 dollars) mails at 80% and when the forecast passes 100%, and `daily_budget_usd` (10 dollars) mails when the day passes it. A demo machine left on costs about 5.30 dollars a day.
- Cold start. With no demo machine running, a visitor waited 270 seconds on 2026-10-02 with no image. `deploy/aws/openshell/bake-cell-image.sh` makes a node image: about 25 minutes, and it writes the image id to `/gusto/poc/cell-ami`. The demo bundle workflow runs it after every merge that changes the key from `bake-key.sh`, and it does nothing when an image of that key exists. A host with an older image installs everything and is slower.
- Node kit. Gusto cloud that starts nodes runs `ansible/cell-kit.yml` once a release. It writes a node's files to `kits/<sha256>.tgz` in the demo's directory of the shared file system, and starts each node with that sum. A node from the matching image mounts the file system, checks the sum, unpacks the kit and starts its services, with no playbook and no download. If the kit is missing, its sum differs or the image differs, the node runs the playbook.
- Releases on running nodes. A node keeps its own release, but the organizations' servers on it run gusto cloud's. Gusto cloud copies its program to `releases/<sum>/gusto` under `GUSTO_CLOUD_ROOT`, on the shared file system, and nodes start servers from there. Each organization's runtime records the command its server was started with. A gusto cloud server that starts and finds a server running with another command stops it and starts it again as its own release. A deploy therefore restarts every organization's server once, and stops the turns running then. The newest three copies are kept, and any copy younger than a day.
- Kept node. A node of the live gusto cloud is an `m8id.xlarge`, which has a disk of its own. The gateway keeps each sandbox and its records there, and AWS blanks that disk whenever the host stops. Gusto cloud keeps one node (`demo_cell_keep`): when it is idle it is stopped, not removed, and when a node is needed it is given the current start-up script and started again. Its root disk holds only the image's files, so a stopped kept node costs that 40 GB disk, about 3.20 dollars a month once the disk is on the baseline throughput. A new disk is read from the image's snapshot a block at a time and needs the launch template's throughput: on the baseline a start-up took 41 s, with the first read of a parameter timing out, against 8.5 s. Gusto cloud lowers the kept node's disk to the baseline when it first stops it. On 2026-10-25 a kept node was running 8.9 s after it was started, and a new one about 17 s after it was launched. When the kept node is busy, a new node is launched. A kept node is launched to stop, not be removed, when it powers itself off at its twelfth hour of uptime or because gusto cloud has not been heard from, so it can be started again; every other node is removed when it powers off. Gusto cloud's scaler lists the stopped nodes each time it decides: a stopped node that is not the kept one, or is beyond `demo_cell_keep`, is removed, and the kept one's disk is lowered to the baseline if it stopped itself and so was not. A kept node made before this is given the same behaviour when it is next started. A stopped kept node with an older image is started like any other and runs the playbook, which is slower and correct.
- Stopping a node. The scaler stops a node that has had nothing on it for `GUSTO_CLOUD_NODE_IDLE_MINUTES` while no organization waits for room or is being placed or started. It marks the node first, so nothing is placed or started on it from its last check, and takes its registration away once it is stopped or removed, so the organizations placed on it are placed again at once. A node that goes stops its organizations' servers together, each given 30 seconds. The day's prewarm count is kept in `prewarms` beside the registrations, so a gusto cloud server that starts again counts on from it.
- No cloud-init. The image of a node has cloud-init turned off. `gusto-launch` runs the host's start-up script on every start, and the network is one for any interface. The image has no sshd and no SSH host keys, since cloud-init made new keys for each host and without it every host would share them; a node is reached through Systems Manager.
