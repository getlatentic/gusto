# Builds the proxy's credentials in the directory given, from Parameter Store, with the
# instance's role: $BEDROCK_PARAMETER and $DEEPSEEK_PARAMETER name the two keys. "platform" and "gemma" are one Bedrock key
# under two names, because Mantle serves Gemma 4 under /openai and the other models under /. Gemma answers Chat Completions
# alone, so Claude Code's and Codex's calls to it are translated. The gemma credential answers every harness: the person chooses
# which engine runs on which model. DEEPSEEK_HARNESSES, when set, is the harnesses the deepseek credential answers, as names
# separated by spaces, and when not set it answers every one. The anthropic credential is DeepSeek's Anthropic-shaped address, which
# only Claude Code speaks.
set -euo pipefail
dir=${1:?the directory to write the credentials in}
mkdir -p "$dir"
python3 - "$BEDROCK_PARAMETER" "$DEEPSEEK_PARAMETER" "$dir" <<'PY'
import json, os, subprocess, sys
def secret(name):
    return subprocess.run(["aws", "ssm", "get-parameter", "--with-decryption", "--name", name,
                           "--query", "Parameter.Value", "--output", "text"],
                          check=True, capture_output=True, text=True).stdout.strip()
bedrock, deepseek = secret(sys.argv[1]), secret(sys.argv[2])
only = os.environ.get("DEEPSEEK_HARNESSES", "").split()
held = {
    "platform": {"address": "https://bedrock-mantle.us-east-1.api.aws", "auth": "bearer", "key": bedrock, "harnesses": ["opencode"]},
    "gemma": {"address": "https://bedrock-mantle.us-east-1.api.aws/openai", "auth": "bearer", "key": bedrock, "wires": ["chat"], "maxOutput": 8192},
    "deepseek": {"address": "https://api.deepseek.com", "auth": "bearer", "key": deepseek, **({"harnesses": only} if only else {})},
    "anthropic": {"address": "https://api.deepseek.com/anthropic", "auth": "anthropic", "key": deepseek, "harnesses": ["claude"]},
}
for name, credential in held.items():
    fd = os.open(os.path.join(sys.argv[3], name + ".json"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as out:
        json.dump(credential, out)
PY
