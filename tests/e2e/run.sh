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

  export GO_DISCOVERY_SEED_DB_FILE=e2e_test_modules.txt
  docker-compose -f devtools/docker/compose.yaml build &&
  docker-compose -f devtools/docker/compose.yaml run seeddb &&
  docker-compose -f devtools/docker/compose.yaml run e2e $files $@

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
