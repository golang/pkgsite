#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

cleanup() {
  docker-compose -f devtools/docker/docker-compose.yaml down --remove-orphans
}

error() {
  echo ""
  echo "---------- ERROR: docker-compose db logs ----------"
  docker-compose -f devtools/docker/docker-compose.yaml logs db
  echo ""
  echo "---------- ERROR: docker-compose migrate logs ----------"
  docker-compose -f devtools/docker/docker-compose.yaml logs migrate
  echo ""
  echo "---------- ERROR: docker-compose frontend logs ----------"
  docker-compose -f devtools/docker/docker-compose.yaml logs frontend
  echo ""
  echo "---------- ERROR: docker-compose chrome logs ----------"
  docker-compose -f devtools/docker/docker-compose.yaml logs chrome
  echo ""
  echo "---------- ERROR: docker-compose e2e logs ----------"
  docker-compose -f devtools/docker/docker-compose.yaml logs e2e
  cleanup
}

main() {
  trap cleanup EXIT
  trap error ERR

  local files="e2e"
  for arg in "$@"; do
    if [[ $arg == e2e/* ]];then
      files=""
    fi
  done

  # GO_DISCOVERY_E2E_ARGS is used to pass arguments to the e2e tests and
  # indicate which tests to run.
  export GO_DISCOVERY_E2E_ARGS="$files $@"
  echo "GO_DISCOVERY_E2E_ARGS=$GO_DISCOVERY_E2E_ARGS"
  docker-compose -f devtools/docker/docker-compose.yaml build &&
  docker-compose -f devtools/docker/docker-compose.yaml run e2e

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
