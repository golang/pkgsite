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
script_dir=$(dirname "$(readlink -f "$0")")
pkgsite_dir=$(readlink -f "${script_dir}/..")

# Run postgres.
pg_container=$(${maybe_sudo}docker run --rm -d -e LANG=C postgres:11.4)
trap "${maybe_sudo} docker stop ${pg_container}" EXIT

# Run all.bash. To avoid any port conflict, run in the postgres network.
cd "${pkgsite_dir}"
${maybe_sudo}docker run --rm -t \
  --network container:${pg_container} \
  -v $(pwd):"/workspace" -w "/workspace" \
  -e GO_DISCOVERY_TESTDB=true golang:1.15 ./all.bash ci
