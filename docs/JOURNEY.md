# The journey

What a person does to get value, end to end. Every sprint is a slice of this,
and at the end of each one somebody can do one more step than before.

1. A developer writes down a change and the command that proves it: a brief and
   a gate.
2. They pick a harness and a model, or take the team's default.
3. The job runs somewhere isolated, and they watch it as it happens.
4. It stops on its own when it passes a budget, rather than running all night.
5. The gate passes, and a pull request opens with the evidence attached.
6. A reviewer reads the evidence and re-checks it without trusting the platform
   that produced it.
7. A platform lead sees what every agent cost last month, what it touched, and
   which harness does better on this codebase.
8. A regulated team runs all of it in their own cloud, on keys that never enter
   a sandbox.

## The slices

| sprint | after it, a person can | ships |
| --- | --- | --- |
| 0 | run one job on their laptop and check the evidence | the CLI, the two formats, the container, one measured adapter |
| 1 | open a URL, start a job, and watch it happen | the service deployed, a sign-in, the pages to submit and watch, the evidence listed |
| 2 | send an agent at a real repository and review what it did | a pull request carrying the evidence, the vendor harnesses, the evidence rendered |
| 3 | account for what it cost, and stop a job that spends too much | the proxy that holds the keys and counts tokens, hard caps, a spend page |
| 4 | ask which harness is better on their own codebase | the same job on two harnesses, compared on one page |
| 5 | run the whole thing, UI included, in their own cloud | one-artifact install, SSO, roles, audit export, a signed record |
| 6 | buy it as a service | tenancy, billing, runner pools, customer-machine runners |

Every sprint from 1 onwards ends with something live at a URL. A sprint that ends
with a binary nobody can open has not shipped.

Sprint 0 is the walking skeleton: the thinnest path that goes all the way
through. Everything after it keeps that path working.
