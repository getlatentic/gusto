# Settings that change while the server runs

A change to what people are offered, or to their limits, takes effect without a restart, so nobody in the middle of a task is sent
away for one. The server looks at the files below every `GUSTO_RELOAD_SECONDS` (20 by default). A change is taken when the same new
content is there at two looks in a row, so a file that an editor has emptied and not yet filled is never the setting: it shows within
a minute at the default.

| Setting | What it holds | What a change does |
| --- | --- | --- |
| `GUSTO_LIVE` | `limitPerson`, `limitTeam` (jobs at once) and `models` | The queue takes the limits from the next job it picks. The pages offer the new models. A field the file leaves out keeps the value the server started with from `GUSTO_LIMIT_PERSON`, `GUSTO_LIMIT_TEAM` and `GUSTO_MODELS`. |
| `GUSTO_SPEND_POLICY` | prices, team caps, the organisation's cap | From the next call. What has been spent is kept, so a cap lowered below it refuses the next call. |
| `GUSTO_TOOL_POLICY` | rules that take tools from callers | A job that is running asks the policy at each call, so a rule reaches it at once. |
| `GUSTO_REDACTION` | the kinds of data to mask (see [GATEWAY.md](GATEWAY.md)) | The next model call, and the next tool answer. A bad file keeps what was masked before. |
| `GUSTO_CREDENTIALS` | a `<name>.json` for each credential | The next model call through the proxy uses the new key and address, including a job that is running. A folder with no credential is refused. |

A file that does not read is refused whole, and what was in force stays. A misspelt field is refused, because a misspelt limit would
otherwise be no limit. The server says so on its log and in the change record, which has a line for every change and every refusal
with a fingerprint of the file before and after, never its contents.

When the organisation has a registry (`GUSTO_REGISTRY`) its administrators change the models, the credentials and the prices in the
console, and those changes were already live. The registry decides them, and the files here do not.

## Read once, when the server starts

A change to these needs a restart, and the change record notes it at the next start:

- where things are kept: `GUSTO_ADDR`, `GUSTO_JOBS`, `GUSTO_SESSIONS`, `GUSTO_SETUPS`, `GUSTO_INTEGRATIONS`, `GUSTO_PRESETS`, `GUSTO_CHANGES`, `GUSTO_RETENTION`
- who may sign in and who is an administrator: `GUSTO_ALLOWED`, `GUSTO_ADMINS`, the groups and the identity provider's settings
- how jobs run: `GUSTO_SANDBOX`, `GUSTO_JOB_NETWORK`, `GUSTO_CONTAINER`, `GUSTO_JOB_CONCURRENCY`, `GUSTO_ALLOWED_HOSTS`
- the tool servers an operator offers (`GUSTO_MCP`) and the gateway's mode and network rule
- the retention policy file
