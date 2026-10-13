#!/bin/sh
# Sets up a haas server on a Linux machine of your own: a VPS, a virtual machine, a box in a rack. It checks the machine, installs
# Docker if the machine has none (Debian and Ubuntu), writes the settings with a password it makes, starts haas, and says how to sign in.
#
#   sudo deploy/install/server.sh --domain agents.example.com --from-source
#   sudo deploy/install/server.sh --no-tls --release v0.1.0-rc.2
#
#   --domain NAME      the name that points at this machine; haas then gets a certificate of its own and is served over HTTPS
#   --tls-internal     with --domain: a certificate of Caddy's own, for trying an install (a browser warns)
#   --no-tls           serve plain HTTP on port 8040, for a machine that only a trusted network reaches
#   --release TAG      which release's images to run; the newest by default
#   --from-source      build the images from the checkout this script is in, and not pull a release's
#   --dir DIR          where the settings and the data are kept; /opt/haas by default
#   --model-url URL --model-key KEY --models a,b    a model provider to start with; add one later in the pages otherwise
#
# Run it again to change a setting or to upgrade: the password and the data stay.
set -eu

domain= internal= notls= release= source= dir=/opt/haas
model_url= model_key= models=
while [ $# -gt 0 ]; do
  case "$1" in
    --domain) domain=$2; shift ;;
    --tls-internal) internal=1 ;;
    --no-tls) notls=1 ;;
    --release) release=$2; shift ;;
    --from-source) source=1 ;;
    --dir) dir=$2; shift ;;
    --model-url) model_url=$2; shift ;;
    --model-key) model_key=$2; shift ;;
    --models) models=$2; shift ;;
    -h | --help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "server.sh: $1 is not an option (--help lists them)" >&2; exit 2 ;;
  esac
  shift
done

fail() { echo "server.sh: $*" >&2; exit 1; }
say() { printf '%s\n' "$*"; }

[ "$(uname -s)" = Linux ] || fail "this installs on Linux; $(uname -s) can run haas for one person with the install script"
[ "$(id -u)" = 0 ] || fail "run it as root: sudo $0 ..."
if [ -z "$domain" ] && [ -z "$notls" ]; then
  fail "choose how people reach it: --domain NAME for HTTPS, or --no-tls for plain HTTP on a trusted network"
fi
[ -n "$domain" ] && [ -n "$notls" ] && fail "--domain and --no-tls are two answers to one question"

case "$(uname -m)" in
  x86_64 | amd64) suffix= ;;
  aarch64 | arm64) suffix=-arm64 ;;
  *) fail "$(uname -m) is a processor no image is built for" ;;
esac

# Docker, and the Compose plugin at 2.24 or later, which is the first to read the env_file form compose.yaml uses.
if ! command -v docker >/dev/null 2>&1; then
  command -v apt-get >/dev/null 2>&1 || fail "Docker is not installed, and this script installs it only with apt: install Docker Engine and the Compose plugin, then run it again"
  say "Installing Docker..."
  DEBIAN_FRONTEND=noninteractive apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io docker-compose-v2 >/dev/null
fi
docker info >/dev/null 2>&1 || fail "Docker is installed and does not answer: systemctl start docker"
docker compose version >/dev/null 2>&1 || fail "the Docker Compose plugin is missing: apt-get install docker-compose-v2"

# Where the compose files are: the checkout this script is in, or the release's bundle.
here=$(cd "$(dirname "$0")" && pwd)
if [ -n "$source" ]; then
  [ -f "$here/../compose/compose.yaml" ] || fail "--from-source needs this script inside a checkout of haas"
  compose=$(cd "$here/../compose" && pwd)
else
  repo=${HAAS_REPO:-getlatentic/haas}
  if [ -z "$release" ]; then
    release=$(curl -fsSL "https://api.github.com/repos/$repo/releases" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1) || true
    [ -n "$release" ] || fail "no release was found for $repo: name one with --release, or build from a checkout with --from-source"
  fi
  mkdir -p "$dir/release"
  bundle="haas-compose_${release}.tar.gz"
  base=${HAAS_RELEASE_URL:-https://github.com/$repo/releases/download/$release}
  curl -fsSL "$base/$bundle" -o "$dir/release/$bundle" ||
    fail "could not download $bundle from release $release of $repo"
  tar -xzf "$dir/release/$bundle" -C "$dir/release"
  compose="$dir/release/compose"
fi

mkdir -p "$dir"
env="$compose/.env"
password=
[ -f "$env" ] && password=$(sed -n 's/^HAAS_PASSWORD=//p' "$env")
[ -n "$password" ] || password=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
[ -f "$env" ] || cp "$compose/env.example" "$env"
chmod 600 "$env"

set_env() {
  if grep -q "^$1=" "$env"; then
    sed -i "s|^$1=.*|$1=$2|" "$env"
  elif grep -q "^# $1=" "$env"; then
    sed -i "s|^# $1=.*|$1=$2|" "$env"
  else
    printf '%s=%s\n' "$1" "$2" >> "$env"
  fi
}
set_env HAAS_PASSWORD "$password"
set_env HAAS_DATA "$dir/data"
set_env HAAS_API_TOKENS "$dir/data/tokens/tokens.json"
[ -n "$model_url" ] && set_env HAAS_MODEL_UPSTREAM "$model_url"
[ -n "$model_key" ] && set_env HAAS_MODEL_KEY "$model_key"
[ -n "$models" ] && set_env HAAS_MODELS "$models"
if [ -z "$source" ]; then
  set_env HAAS_SERVER_IMAGE "ghcr.io/getlatentic/haas-server:${release}${suffix}"
  set_env HAAS_RUNNER_IMAGE "ghcr.io/getlatentic/haas-runner:${release}${suffix}"
fi

profile=
if [ -n "$domain" ]; then
  set_env HAAS_PUBLISH "127.0.0.1:8040"
  set_env HAAS_DOMAIN "$domain"
  [ -n "$internal" ] && set_env HAAS_CADDY_TLS "tls internal"
  profile="--profile tls"
  address="https://$domain"
else
  set_env HAAS_PUBLISH "0.0.0.0:8040"
  address="http://$(hostname -I 2>/dev/null | awk '{print $1}'):8040"
fi

cd "$compose"
say "Starting haas..."
if [ -n "$source" ]; then
  docker compose $profile up -d --build
else
  docker compose $profile up -d --no-build
fi

port=8040
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1 || docker compose exec -T server bash -c "exec 3<>/dev/tcp/127.0.0.1/8040" >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
[ -n "${ready:-}" ] || fail "haas did not answer: docker compose -f $compose/compose.yaml logs server"

cat <<EOS

haas is running.

  Open     $address
  Sign in  with the password:  $password
  Kept in  $env (readable by root only)

To use it from your computer:
  haas-helper login -server $address     (the code it shows is approved in the page above)
  or the desktop app: Connect to an organisation, and type $address

Nothing is sent anywhere but to the model provider you add. The server holds this machine's Docker socket, which is root on it:
keep nothing else of value here (deploy/README.md says why).
EOS
