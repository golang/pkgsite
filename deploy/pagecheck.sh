#!/usr/bin/env -S bash -e

# Copyright 2022 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Structural HTML checks against an endpoint.
# Examples:
#    pagecheck.sh staging

if [[ $(basename $PWD) != private ]]; then
  cd private
fi

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  cat >&2 <<END

  Usage: $0 [exp|dev|staging|prod|beta] IDTOKEN

  Run the pagecheck tests against the given environment

END
  exit 1
}

main() {
  local env=$1
  local idtok=$2
  check_env $env

  case $env in
    exp|dev|staging)
      if [ -z $idtok ]; then
        idtok=$(cat ../_ID_TOKEN)
      fi
      if [[ $idtok == '' ]]; then
        die "need idtoken for $env"
      fi
      ;;
  esac

  local credsarg=''
  if [[ $idtok != '' ]]; then
    credsarg="-idtok $idtok"
  fi

  testpath='./devtools/pagecheck_test'
  runcmd go test -v $testpath -base $(frontend_url $env) $credsarg
}

main "$@"
