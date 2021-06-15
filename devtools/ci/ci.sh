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
./devtools/nodejs.sh npm install --quiet
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
echo "Running postgres"
echo "----------------------------------------"
pg_container=$(${maybe_sudo}docker run --rm -d -e LANG=C postgres:11.4)
trap "${maybe_sudo} docker stop ${pg_container}" EXIT
print_duration_and_reset

echo "----------------------------------------"
echo "Running all.bash"
echo "----------------------------------------"
${maybe_sudo}docker run --rm -t \
  --network container:${pg_container} \
  -v $(pwd):"/workspace" -w "/workspace" \
  -e GO_DISCOVERY_TESTDB=true golang:1.15 ./all.bash ci
print_duration_and_reset

echo "----------------------------------------"
echo "Running e2e tests"
echo "----------------------------------------"
# TODO: add ./devtools/run_e2e.sh
print_duration_and_reset

echo
echo "----------------------------------------"
