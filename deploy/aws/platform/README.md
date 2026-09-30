# The public cloud demo's AWS pieces, and how a host is made

A demo host is made and removed by workflows, so nothing runs from a laptop. This directory is what
outlives a host, and the script a host starts with.

| Piece | What it is |
| --- | --- |
| `main.tf` | The role and profile every host has, the launch template a host is made from, the security group (https only), and the role GitHub Actions takes. |
| `bootstrap.sh` | Applies `main.tf`, from a machine with the rights to make IAM roles, and prints the repository variables to set. Rare. |
| `user-data.sh` | What a host runs when it starts: it fetches the release bundle, checks its checksum, and configures itself. |

## The path

1. A merge to `main` that changes what a host is made from runs **demo bundle**. It puts `bundle.tgz` (the Linux `haas`, its DNS program and the playbooks) in the store bucket under `releases/<commit>/`, and builds the runner image into `image/<key>.tar.gz` when that key is not there.
2. **demo up** (a button, with the lease in minutes) launches a host from the launch template with `user-data.sh`. The host checks the bundle's checksum, runs the playbooks on itself, gets the certificate and the image from the bucket, points the name at itself, and starts the front door. The workflow says when `https://<name>/healthz` answers.
3. The host removes itself when its lease ends. **demo down** tells it to power off now. **demo sweep** runs every 30 minutes and terminates anything tagged `haas-poc` that has been up more than six hours.

The CI role can launch the launch template, tell tagged hosts to power off, terminate tagged hosts, and put
objects under `releases/` and `image/`. It has no IAM right, so it cannot widen itself.

A host that fails to start powers off 15 minutes after the failure, and every host has a timer for the most it
may be up. There is no SSH: `aws ssm start-session --target <instance id>` is how a person reaches one.

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
```
