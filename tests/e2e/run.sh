#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

main() {
  trap docker_cleanup EXIT
  trap docker_error ERR

  local files="e2e --runInBand"
  for arg in "$@"; do
    if [[ $arg == * ]];then
      files=""
    fi
  done

  export GO_DISCOVERY_CONFIG_DYNAMIC=tests/e2e/config.yaml
  export GO_DISCOVERY_DATABASE_NAME=discovery_e2e_test
  export GO_DISCOVERY_SEED_DB_FILE=tests/e2e/seed.txt
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
