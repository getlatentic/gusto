# Builds the proxy's credentials in the directory given, from Parameter Store, with the
# instance's role: $BEDROCK_PARAMETER and $DEEPSEEK_PARAMETER name the two keys. "platform" and "gemma" are one Bedrock key
# under two names, because Mantle serves Gemma 4 under /openai and the other models under /. Gemma answers Chat Completions
# alone, so Claude Code's and Codex's calls to it are translated. DeepSeek is one credential at two addresses: OpenAI's shapes at
# its root and Anthropic's under /anthropic, so every engine reaches its models without translation.
# Every credential is offered to every engine: the person chooses which engine runs on which model.
# READ_PARAMETER is a command that prints a parameter given its name, as "gusto param"; the AWS command reads them when it is not set.
set -euo pipefail
dir=${1:?the directory to write the credentials in}
mkdir -p "$dir"
python3 - "$BEDROCK_PARAMETER" "$DEEPSEEK_PARAMETER" "$dir" <<'PY'
import json, os, shlex, subprocess, sys
reader = shlex.split(os.environ.get("READ_PARAMETER", ""))
def secret(name):
    command = reader + [name] if reader else ["aws", "ssm", "get-parameter", "--with-decryption", "--name", name,
                                              "--query", "Parameter.Value", "--output", "text"]
    return subprocess.run(command, check=True, capture_output=True, text=True).stdout.strip()
bedrock, deepseek = secret(sys.argv[1]), secret(sys.argv[2])
held = {
    "platform": {"address": "https://bedrock-mantle.us-east-1.api.aws", "auth": "bearer", "key": bedrock},
    "gemma": {"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": bedrock, "wires": ["chat"], "maxOutput": 8192},
    "deepseek": {"address": "https://api.deepseek.com", "auth": "bearer", "key": deepseek,
                 "endpoints": [{"address": "https://api.deepseek.com/anthropic", "auth": "anthropic"}]},
}
for name, credential in held.items():
    fd = os.open(os.path.join(sys.argv[3], name + ".json"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as out:
        json.dump(credential, out)
PY
