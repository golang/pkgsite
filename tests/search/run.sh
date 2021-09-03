#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

set -e

main() {
  trap docker_cleanup EXIT
  trap docker_error ERR

  # These variables are used by the seedddb script.
  export GO_DISCOVERY_DATABASE_NAME=discovery_symbol_test
  export GO_DISCOVERY_CONFIG_DYNAMIC=tests/search/config.yaml
  export GO_DISCOVERY_SEED_DB_FILE=tests/search/seed.txt
  dockercompose build && dockercompose run --rm seeddb && dockercompose run --rm searchtest

  local status=$?
  if [ $status -eq 0 ]
  then
    echo "Done!"
  else
    echo "Search tests failed."
  fi
  exit $status
}

main $@
