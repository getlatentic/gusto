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
| 1 | finish the loop: a pull request carrying the evidence | `haas ship`, the vendor adapters, tool calls in the record |
| 2 | send a job to a server and watch it from somewhere else | `haasd`, a queue, a live stream, an evidence store |
| 3 | account for what it cost, and stop a job that spends too much | the proxy that holds the keys and counts tokens, hard caps |
| 4 | ask which harness is better on their own codebase | the same job on two harnesses, verified side by side |
| 5 | run the whole thing in their own cloud | one-artifact install, SSO, roles, audit export, signed evidence |
| 6 | buy it as a service | tenancy, billing, runner pools, customer-machine runners |

Sprint 0 is the walking skeleton: the thinnest path that goes all the way
through. Everything after it keeps that path working.
