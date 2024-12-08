#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [OPTIONS] [exp|dev|staging|prod|beta] [IDTOKEN]

  Run the screentest check against a live instance of the given environment.
  These tests will only pass against staging and prod.
  If IDTOKEN is omitted, it is read from the file _ID_TOKEN.

  Options:
    -neverfail
      Do not return a non-zero exit code, even if the screentests fail.
      This causes Cloud Build to upload screentest results for debugging.
      It should never be used as part of a normal deployment.

EOUSAGE
  exit 1
}

main() {
  if [[ $# = 0 ]]; then
    usage
  fi
  neverfail=false
  if [[ $1 = -neverfail ]]; then
    neverfail=true
    shift
  fi

  local env=$1
  local idtoken=$2
  check_env $env
  if [ -z $idtoken ]; then
    idtoken=$(cat _ID_TOKEN)
  fi
  ./tests/screentest/run.sh -idtoken $idtoken -concurrency 1 $env || $neverfail
  # If the tests passed, the tests/screentest/output directory will be empty.
  # That causes the "artifacts" part of deploy.yaml to fail, because Cloud Build
  # can't find any files there to upload. So create something there.
  outdir=tests/screentest/output
  if [[ ! -d $outdir ]]; then
    mkdir -p $outdir/testcases
    touch $outdir/success
    touch $outdir/testcases/success
  fi
}

main $@
