#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [exp|dev|staging|prod|beta] IDTOKEN

  Run the screentest check against a live instance of the given environment.
  These tests will only pass against staging and prod.

EOUSAGE
  exit 1
}

main() {
  local env=$1
  local idtoken=$2
  check_env $env
  if [ -z $idtoken ]; then
    idtoken=$(cat _ID_TOKEN)
  fi
  ./tests/screentest/run.sh --idtoken $idtoken --concurrency 1 $env
}

main $@
