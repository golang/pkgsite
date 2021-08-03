#!/usr/bin/env bash
# Copyright 2020 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e

usage() {
  cat <<EOUSAGE
Usage: $0 [--sudo]

Run standard CI (tests and linters) using local docker. If --sudo is set, run
docker with sudo.
EOUSAGE
}

# Run tools in continuous integration mode.
export CI=true
# Skip installing chrome with puppeteer. We are using the browserless/chrome
# docker image instead.
export PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
# Set the database name to be used in docker-compose.
export GO_DISCOVERY_DATABASE_NAME=discovery_ci_test

# starttime is the start time for this entire script.
starttime=`date +%s`
# sectionstart is the start time for a section. This is reset in
# print_duration_and_reset.
sectionstart=$starttime
# print_duration_and_reset prints the duration since sectionstart and since
# starttime. It also resets sectionstart to the current time.
print_duration_and_reset() {
  local end=`date +%s`

  # Print how long it took this section of CI.
  echo
  echo "DONE: $((end-sectionstart)) seconds ($((end-starttime)) seconds so far)"
  sectionstart=$end # Reset startime to measure the next CI section.
}

echo "----------------------------------------"
echo "Starting CI"
echo "----------------------------------------"
maybe_sudo=
while [[ $# -gt 0 ]]; do
  case "$1" in
    "-h" | "--help" | "help")
      usage
      exit 0
      ;;
    "--sudo")
      maybe_sudo="sudo "
      shift
      ;;
    *)
      usage
      exit 1
  esac
done

# Find the repo root.
script_dir=""
pkgsite_dir=""
if [[ "$OSTYPE" == "darwin"* ]]; then
  # readlink doesn't work on Mac. Replace with greadlink for testing locally.
  # greadlink can be installed with brew install coreutils.
  script_dir=$(dirname "$(greadlink -f "$0")")
  pkgsite_dir=$(greadlink -f "${script_dir}/../..")
else
  script_dir=$(dirname "$(readlink -f "$0")")
  pkgsite_dir=$(readlink -f "${script_dir}/../..")
fi

# Run all.bash. To avoid any port conflict, run in the postgres network.
cd "${pkgsite_dir}"
print_duration_and_reset

echo "----------------------------------------"
echo "Installing NPM"
echo "----------------------------------------"
./devtools/nodejs.sh npm ci
print_duration_and_reset

echo "----------------------------------------"
echo "Running CSS/JS linters"
echo "----------------------------------------"
./devtools/nodejs.sh npm run lint
print_duration_and_reset

echo "----------------------------------------"
echo "Running JS tests"
echo "----------------------------------------"
./devtools/nodejs.sh npm run test
print_duration_and_reset

echo "----------------------------------------"
echo "Running all.bash"
echo "----------------------------------------"
./devtools/docker/compose.sh run allbash ci
print_duration_and_reset

echo "----------------------------------------"
echo "Running e2e tests"
echo "----------------------------------------"
./tests/e2e/run.sh
print_duration_and_reset

echo "----------------------------------------"
echo "Running search tests"
echo "----------------------------------------"
./tests/search/run.sh
print_duration_and_reset

echo
echo "----------------------------------------"
