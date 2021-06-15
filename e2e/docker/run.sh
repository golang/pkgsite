#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

cleanup() {
  docker-compose -f e2e/docker/docker-compose.yaml down --remove-orphans
}

error() {
  echo "---------- ERROR: docker-compose logs ----------"
  docker-compose -f e2e/docker/docker-compose.yaml logs
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

  docker-compose -f e2e/docker/docker-compose.yaml build &&
  docker-compose -f e2e/docker/docker-compose.yaml run e2e

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
