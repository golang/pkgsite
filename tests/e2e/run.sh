#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

getfile() {
  local file
  if [[ -d $1 && -e "$1/$2" ]]; then
    file="$1/$2"
  elif [[ -f $1 && -e "$(dirname $1)/$2" ]]; then
    file="$(dirname $1)/$2"
  fi
  echo $file
}

main() {
  trap docker_cleanup EXIT
  trap docker_error ERR

  local files="tests/e2e/*.test.ts --runInBand"
  local config
  local seed
  for arg in "$@"; do
    if [[ $arg == tests/* ]]; then
      files=""
    fi
    config=$(getfile $arg config.yaml)
    seed=$(getfile $arg seed.txt)
    if [[ $config != "" || $seed != "" ]]; then
      break
    fi
  done

  export GO_DISCOVERY_CONFIG_DYNAMIC=${config:-"tests/e2e/config.yaml"}
  export GO_DISCOVERY_DATABASE_NAME=discovery_e2e_test
  export GO_DISCOVERY_SEED_DB_FILE=${seed:-"tests/e2e/seed.txt"}
  dockercompose build && dockercompose run seeddb && dockercompose run e2e $files $@

  local status=$?
  if [ $status -eq 0 ]
  then
    echo "Done!"
  else
    echo "e2e tests failed."
  fi
  exit $status
}

main $@
