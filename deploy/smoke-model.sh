#!/bin/sh
# One job whose harness asks a model through the proxy, and a check that the proxy
# counted what the model said it used. The install points a credential at
# deploy/stubmodel/stub.py, which always answers 1000 tokens in and 100 out, and
# prices that model at 1000 and 10000 dollars a million, so the cost is 2 dollars.
# Run it as deploy/smoke.sh is run, inside the server:
#
#   docker compose exec -T server sh < ../smoke-model.sh
#
# The harness is a fixture that makes one call and nothing else, so a pass proves
# the proxy holds the key, reaches the model and counts the call, and says nothing
# about any real model.
set -eu

data=$(dirname "$HAAS_JOBS")
smoke="$data/smoke"
rm -rf "$smoke/model-job"
mkdir -p "$smoke/model-repo" "$smoke/model-harnesses"

cd "$smoke/model-repo"
if [ ! -d .git ]; then
  echo "nothing to change" > README
  git init -q . && git add -A
  git -c user.email=haas@localhost -c user.name=haas commit -qm "a repository to ask from"
fi

cat > "$smoke/model-harnesses/modelfixture.json" <<'JSON'
{
  "name": "modelfixture",
  "program": "node",
  "argv": ["-e", "fetch(process.env.HAAS_MODEL_URL + '/chat/completions', {method: 'POST', headers: {'content-type': 'application/json', authorization: 'Bearer ' + process.env.HAAS_MODEL_TOKEN}, body: JSON.stringify({model: 'stub-model', messages: [{role: 'user', content: 'hi'}]})}).then(r => r.text()).then(t => console.log(t))"],
  "env": {"HAAS_MODEL_URL": "{modelUrl}", "HAAS_MODEL_TOKEN": "{modelToken}"},
  "needs": "key",
  "stream": "plain"
}
JSON

HAAS_HARNESSES="$smoke/model-harnesses" haas new "$smoke/model-job" -repo "$smoke/model-repo" \
  -brief "ask the model" -harness modelfixture -model stub/stub-model -- true >/dev/null
if [ -n "${HAAS_API_TOKENS:-}" ] && [ -z "${HAAS_TOKEN:-}" ]; then
  echo "this server takes API tokens: set HAAS_TOKEN to one that may send jobs" >&2
  exit 2
fi
id=$(HAAS_SERVER=http://127.0.0.1:8040 haas submit "$smoke/model-job" | head -n 1)
echo "submitted $id"

tries=0
until [ -f "$HAAS_JOBS/$id/out/manifest.json" ]; do
  tries=$((tries + 1))
  if [ "$tries" -gt 120 ]; then
    echo "no manifest after 120 seconds; the job page says why: /jobs/$id" >&2
    exit 1
  fi
  sleep 1
done
haas verify "$HAAS_JOBS/$id" || true

calls="$HAAS_JOBS/$id/out/proxy.jsonl"
echo "the proxy's record:"
cat "$calls"
for want in '"inputTokens":1000' '"outputTokens":100' '"usd":2'; do
  grep -q "$want" "$calls" || { echo "the proxy's record does not say $want" >&2; exit 1; }
done
echo "the proxy counted 1000 tokens in, 100 out and 2 dollars"
