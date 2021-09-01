#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

set -e

main() {
  trap docker_cleanup EXIT
  trap docker_error ERR

  export GO_DISCOVERY_CONFIG_DYNAMIC=""
  export GO_DISCOVERY_DATABASE_NAME=discovery_api_test
  export GO_DISCOVERY_SEED_DB_FILE=tests/api/seed.txt
  dockercompose build && dockercompose run seeddb && dockercompose run api

  local status=$?
  if [ $status -eq 0 ]
  then
    echo "Done!"
  else
    echo "API tests failed."
  fi
  exit $status
}

main $@
