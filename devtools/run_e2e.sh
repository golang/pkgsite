#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

cleanup() {
  docker-compose -f devtools/docker/compose.yaml down --remove-orphans
}

error() {
  echo ""
  echo "---------- ERROR: docker-compose db logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs db
  echo ""
  echo "---------- ERROR: docker-compose migrate logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs migrate
  echo ""
  echo "---------- ERROR: docker-compose seeddb logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs seeddb
  echo ""
  echo "---------- ERROR: docker-compose frontend logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs frontend
  echo ""
  echo "---------- ERROR: docker-compose chrome logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs chrome
  echo ""
  echo "---------- ERROR: docker-compose e2e logs ----------"
  docker-compose -f devtools/docker/compose.yaml logs e2e
  cleanup
}

main() {
  trap cleanup EXIT
  trap error ERR

  local files="e2e --runInBand"
  for arg in "$@"; do
    if [[ $arg == e2e/* ]];then
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
