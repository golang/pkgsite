#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

outfile="/tmp/e2e-chrome-$$.log"
start_browser() {
  # trap "kill 0" EXIT # kill the browser process on exit

  docker-compose -f e2e/docker/docker-compose.yaml up -d chrome >& $outfile &
  echo "Starting browser, output at $outfile"
  sleep 30

  # Wait for the browser to start up.
  while ! curl -s http://localhost:3000 > /dev/null; do
    sleep 1
  done
  echo "Browser is up."
}

main() {
  start_browser

  local files="e2e"
  for arg in "$@"; do
    if [[ $arg == e2e/* ]];then
      files=""
    fi
  done

  # Find the repo root.
  script_dir=""
  pkgsite_dir=""
  if [[ "$OSTYPE" == "darwin"* ]]; then
    # readlink doesn't work on Mac. Replace with greadlink.
    script_dir=$(dirname "$(greadlink -f "$0")")
    pkgsite_dir=$(greadlink -f "${script_dir}/../..")
  else
    script_dir=$(dirname "$(readlink -f "$0")")
    pkgsite_dir=$(readlink -f "${script_dir}/../..")
  fi

  cd "${pkgsite_dir}"
  GO_DISCOVERY_E2E_BASE_URL="http://frontend:8080"
  (./devtools/ci/nodejs.sh npx jest $files $@)
  echo "Done!"

  echo "----- Contents of $outfile -----"
  cat $outfile
  docker-compose -f e2e/docker/docker-compose.yaml stop
}

main $@
