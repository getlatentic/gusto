#!/bin/sh
# One real job through a running install: submitted to the server over its API,
# run in a container the server starts from the runner image, and its evidence
# verified. It is the check deploy/ansible's smoke role makes on the box, made
# against compose or Kubernetes instead.
#
# It runs inside the server's container, so it needs nothing where you type but
# the command that reaches in, and the paths it makes are the ones the server
# sees:
#
#   docker compose exec -T server sh < ../smoke.sh                  # compose
#   kubectl -n gusto exec -i deploy/gusto -c server -- sh < deploy/smoke.sh
#
# The agent is a fixture, a program that makes the change and calls no model,
# so a pass proves the install and says nothing about any model.
set -eu

data=$(dirname "$GUSTO_JOBS")
smoke="$data/smoke"
rm -rf "$smoke/job"
mkdir -p "$smoke/repo" "$smoke/harnesses"

# A repository whose test fails until the task fixes it.
cd "$smoke/repo"
if [ ! -d .git ]; then
  cat > adder.sh <<'EOF'
add() {
  echo $(($1 - $2))
}
EOF
  cat > test.sh <<'EOF'
#!/bin/sh
. ./adder.sh
[ "$(add 2 3)" = 5 ] && { echo ok; exit 0; }
echo "add 2 3 gave $(add 2 3), not 5"
exit 1
EOF
  git init -q . && git add -A
  git -c user.email=gusto@localhost -c user.name=gusto commit -qm "an adder that subtracts"
fi

# Where only this check looks, so the pages never offer the fixture. The job
# carries it to the server, the way any adapter of your own travels.
cat > "$smoke/harnesses/fixture.json" <<'EOF'
{
  "name": "fixture",
  "program": "sh",
  "argv": ["-c", "sed -i s/-/+/ adder.sh && echo changed the operator; echo forged-by-the-harness >> /job/out/transcript.log 2>/dev/null; true"],
  "needs": "nothing",
  "stream": "plain"
}
EOF

GUSTO_HARNESSES="$smoke/harnesses" gusto new "$smoke/job" -repo "$smoke/repo" \
  -brief "make add add" -agent fixture -- sh test.sh >/dev/null
# A server that takes API tokens does not take the shared password, so the check
# needs a token that may submit: make one at /tokens and set GUSTO_TOKEN.
if [ -n "${GUSTO_API_TOKENS:-}" ] && [ -z "${GUSTO_TOKEN:-}" ]; then
  echo "this server takes API tokens: set GUSTO_TOKEN to one that may send tasks" >&2
  exit 2
fi
id=$(GUSTO_SERVER=http://127.0.0.1:8040 gusto submit "$smoke/job" | head -n 1)
echo "submitted $id"

# The manifest is the last thing a run writes, so its presence is the end.
tries=0
until [ -f "$GUSTO_JOBS/$id/out/manifest.json" ]; do
  tries=$((tries + 1))
  if [ "$tries" -gt 120 ]; then
    echo "no manifest after 120 seconds; the task page says why: /jobs/$id" >&2
    exit 1
  fi
  sleep 1
done
gusto show "$GUSTO_JOBS/$id"
echo
gusto verify "$GUSTO_JOBS/$id"

# The fixture tried to write into the record; the runner keeps out/ to itself.
if grep -q forged-by-the-harness "$GUSTO_JOBS/$id/out/transcript.log"; then
  echo "the agent wrote into its own record" >&2
  exit 1
fi
