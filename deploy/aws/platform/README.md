# The public cloud demo's AWS pieces, and how a host is made

A demo host is made and removed by workflows, so nothing runs from a laptop. This directory is what
outlives a host, and the script a host starts with.

| Piece | What it is |
| --- | --- |
| `main.tf` | The role and profile every host has, the launch templates a host is made from, the security group (https from anywhere, and the demo's own hosts to each other), the file system the hosts of a demo share, and the role GitHub Actions takes. |
| `bootstrap.sh` | Applies `main.tf`, from a machine with the rights to make IAM roles, and prints the repository variables to set. Rare. |
| `user-data.sh` | What a host runs when it starts: it fetches the release bundle, checks its checksum, and configures itself. |

## The path

1. A merge to `main` that changes what a host is made from runs **demo bundle**. It puts `bundle.tgz` (the Linux `gusto`, its DNS program and the playbooks) in the store bucket under `releases/<commit>/`, and builds the runner image into `image/<key>.tar.gz` when that key is not there.
2. **demo up** (a button, with the lease in minutes) launches a host from the launch template with `user-data.sh`. The host checks the bundle's checksum, runs the playbooks on itself, gets the certificate and the image from the bucket, points the name at itself, and starts gusto cloud. The workflow says when `https://<name>/healthz` answers.
3. The host removes itself when its lease ends. **demo down** tells it to power off now. **demo sweep** runs every 30 minutes and terminates anything tagged `gusto-poc` that has been up more than six hours.

The CI role can launch the launch template, tell tagged hosts to power off, terminate tagged hosts, and put
objects under `releases/` and `image/`. It has no IAM right, so it cannot widen itself.

A host that fails to start powers off 15 minutes after the failure, and every host has a timer for the most it
may be up. There is no SSH: `aws ssm start-session --target <instance id>` is how a person reaches one.

## More than one node

**demo up** with `cells` from 1 to 4 makes gusto cloud (the `orchestrator` template, `t3.medium`, no computers) and that
many nodes that run the organizations' servers (the `cell` template, `m8i.xlarge`, nested virtualization). One node is
made with `cells` at 0, as before.

- The nodes of one demo share an encrypted file system, mounted at `/var/lib/gusto-cloud` on each, in a directory of it
  named for the run. gusto cloud starts an organization's server on a node through the node's agent
  (`gusto host`, port 8060), and the server reads and writes its data by the path gusto cloud knows it by.
  gusto cloud removes the directories of earlier runs when it starts, since the computers they describe went with their nodes.
- A node registers in `cells/` of that directory every ten seconds. A new organization is placed on the node with the
  most free memory, and stays there. When its node has been silent for five minutes it is placed on another node, and its
  people start it again from gusto cloud.
- gusto cloud writes `cells/orchestrator.beat` every ten seconds. A node that has not seen it for five minutes powers
  itself off (and so terminates), after gusto cloud has had 15 minutes to start. A node has no lease of its own.
- Requests to a node are signed with `/gusto/poc/cell-secret`, which `../openshell/put-cell-secret.sh` makes once. A node
  reads it with its role, and nothing else can reach port 8060, which only the security group's own members may.

## gusto cloud that stays up

`aws_instance.live` is a small host (`t3.micro` with a 10 GB disk, and a fixed address) that runs the public demo all the time: nginx and
the certificate, gusto cloud at `https://<name>`, and the scaler that starts a node for an organization that
waits for one and stops a node that has had nothing on it for ten minutes. With nobody using it there is no such node and
nothing is paid for but this machine, its address and the file system.

- It is made once by `bootstrap.sh` and starts itself from the newest release in the bucket. Every **demo bundle** run
  then brings it to that release (an SSM command that runs the start-up script again), which restarts gusto cloud
  and leaves the nodes and the organizations on them as they are.
- Its role is its own, and is the only one that may start and stop nodes. It starts nodes only from the
  `cell-live` template, which tags them `project=gusto-live`, and stops only nodes with that tag that it started
  (`scaler=on`). A node that runs other people's code can start nothing. The demo workflows tag and stop
  `project=gusto-poc`, so they never touch it, and it never touches theirs.
- It has no lease and no limit on how long it runs. What it keeps is on the file system and in the bucket, so it may be
  replaced.
- Sign-up for a visitor is at `https://<name>/demo`. `DEMO_VISITOR_MINUTES` and `DEMO_BOOK_URL` in `demo.env`, stored
  with `../openshell/put-demo-config.sh`, turn it on and say where a call is booked.
- The `demo up` workflow makes a separate, short-lived demo with its own name in the same way as before. Both use the
  same name, so only one of them is up at a time: take the live gusto cloud down (`aws ec2 stop-instances`) before
  running `demo up`.

## Once

GitHub's identity provider must be in the AWS account before the workflows can sign in. IAM does not check that
one exists when the CI role is made, so a missing one shows only when a workflow fails with "The web identity
token could not be validated". Check, and make it when it is not there, as an identity that may (CloudShell in the
console, or an admin profile):

```bash
aws iam get-open-id-connect-provider --open-id-connect-provider-arn arn:aws:iam::<account id>:oidc-provider/token.actions.githubusercontent.com
aws iam create-open-id-connect-provider --url https://token.actions.githubusercontent.com --client-id-list sts.amazonaws.com
```

With an admin profile, `MANAGE_GITHUB_PROVIDER=true ./bootstrap.sh` makes it as part of the stack instead. Do one or the
other, not both. Then the rest:

```bash
./bootstrap.sh                                  # this directory
../openshell/put-cloudflare.sh                  # the two Cloudflare tokens
../openshell/put-demo-config.sh                 # what a host reads that is not secret, from demo.env
../openshell/put-cell-secret.sh                 # the secret nodes are told what to do with; only for demos with several nodes
```
