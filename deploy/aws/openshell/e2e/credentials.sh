# Builds the proxy's credentials in the directory given, from Parameter Store, with the
# instance's role: $BEDROCK_PARAMETER and $DEEPSEEK_PARAMETER name the two keys. "platform" and "oss" are one Bedrock key
# under two names, so that each model is offered only to the harnesses that finished a job on it. Which
# harnesses each credential answers is what greencloud's host_vars say.
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
held = {
    "platform": {"address": "https://bedrock-mantle.us-east-1.api.aws", "auth": "bearer", "key": bedrock, "harnesses": ["opencode"]},
    "oss": {"address": "https://bedrock-mantle.us-east-1.api.aws", "auth": "bearer", "key": bedrock, "harnesses": ["pi", "hermes"]},
    "deepseek": {"address": "https://api.deepseek.com", "auth": "bearer", "key": deepseek},
    "anthropic": {"address": "https://api.deepseek.com/anthropic", "auth": "anthropic", "key": deepseek, "harnesses": ["claude"]},
}
for name, credential in held.items():
    fd = os.open(os.path.join(sys.argv[3], name + ".json"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as out:
        json.dump(credential, out)
PY
