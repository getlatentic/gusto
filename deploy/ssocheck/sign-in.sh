#!/bin/sh
# Signs in through the Dex of deploy/ssocheck/dex.yaml and says what the install did
# with the person.
#
#   deploy/ssocheck/sign-in.sh http://127.0.0.1:8040 admin     # let in as an admin
#   deploy/ssocheck/sign-in.sh http://127.0.0.1:8040 refused    # turned away
#
# Dex answers on the name `dex`, which the install reaches by its network and this
# script reaches on the port Dex publishes to this machine.
set -eu
base=${1:?the address the pages answer on}
want=${2:?admin or refused}
jar=$(mktemp)
trap 'rm -f "$jar"' EXIT

landed=$(curl -s -L --resolve dex:5556:127.0.0.1 -c "$jar" -b "$jar" -o /dev/null -w '%{url_effective}' "$base/auth/start")
admin=$(curl -s -b "$jar" -o /dev/null -w '%{http_code}' "$base/admin/records")
home=$(curl -s -b "$jar" -o /dev/null -w '%{http_code}' "$base/")
echo "signed in through Dex and landed on $landed; /admin/records answered $admin, / answered $home"

case "$want" in
admin)
  [ "$admin" = 200 ] || { echo "the person was not let in as an admin" >&2; exit 1; }
  ;;
refused)
  case "$landed" in *refused=*) ;; *) echo "the person was not refused" >&2; exit 1 ;; esac
  [ "$home" = 303 ] || { echo "a refused person reached the home page: $home" >&2; exit 1; }
  ;;
*) echo "want admin or refused" >&2; exit 2 ;;
esac
echo "as expected: $want"
