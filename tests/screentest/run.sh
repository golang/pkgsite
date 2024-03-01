#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -e

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

# This should match the version we are using in devtools/docker/compose.yaml.
chromedp_version=97.0.4692.71

usage() {
  >&2 cat <<EOUSAGE
Usage: $0 [OPTIONS] [ci|local|exp|dev|staging|prod]

  [ci]
    Run tests against a local server with a seeded database. This is what runs in
    CI/kokoro and should always pass on master.

  [local]
    Run tests against a local server started with ./devtools/run_local.sh <env>
    frontend.

  [exp|dev|staging|prod]
    Run the tests against live instance of the given env. Use to verify that there
    are no unexpected changes after a deploy is complete.

Options:

  --concurrency <N>
    Set the number of testcases to run concurrently. Defaults to 1. Setting this too
    high in lower memory environments may cause instability in tests.

  --update
    Recapture every snapshot during this test run.

  --rm
    Remove containers and volumes after tests are finished.

EOUSAGE
  exit 1
}

dcompose() {
  msg="$@"
  # Scrub Token from output.
  if [[ $msg == *",Token:"* ]]; then
    msg="${msg%*,Token* *},Token:<redacted> ${msg#*,Token* *}"
  fi
  local cmd="docker compose -f devtools/docker/compose.yaml"
  info "\$ $cmd $msg"
  $cmd "$@"
}

cleanup() {
  dcompose stop
  if [ "$rm" = true ]; then
    dcompose down --volumes --remove-orphans
  fi
  if [ ! -z $chromedp ]; then
    runcmd docker container stop $chromedp
  fi
}

main() {
  trap cleanup EXIT
  local concurrency
  local idtoken
  local update
  while [[ $1 = -* ]]; do
    case "$1" in
      "--concurrency"|"-c")
        shift
        concurrency="-c $1"
        ;;
     "--idtoken")
        shift
        idtoken=$1
        ;;
      "--seeddb")
        echo "the seeddb flag is deprecated."
        ;;
      "--update"|"-u")
        update="-u"
        ;;
      "--rm")
        rm=true
        ;;
      *)
        usage
        exit 1
    esac
    shift
  done

  local env=$1
  local debugger_url="-d ws://localhost:9222"
  local vars
  case $env in
    ci)
      debugger_url="-d ws://chromedp:9222"
      vars="-v Origin:http://frontend:8080"
      ;;
    local)
      vars="-v Origin:http://localhost:8080"
      ;;
    exp|dev|staging)
      debugger_url="-d ws://chromedp:9222"
      vars="-v Origin:https://$env-pkg.go.dev,QuotaBypass:$GO_DISCOVERY_E2E_QUOTA_BYPASS,Token:$idtoken"
      ;;
    prod)
      vars="-v Origin:https://pkg.go.dev,QuotaBypass:$bypass"
      ;;
    *)
      usage
      ;;
  esac

  local testfile="tests/screentest/testcases.txt"
  local cmd="screentest $concurrency $debugger_url $vars $update $testfile"

  if [[ "$env" = ci ]]; then
    testfile="'tests/screentest/testcases.*'"
    cmd="screentest $concurrency $debugger_url $vars $update $testfile"
    export GO_DISCOVERY_CONFIG_DYNAMIC="tests/screentest/config.yaml"
    export GO_DISCOVERY_DATABASE_NAME="discovery_e2e_test"
    export GO_DISCOVERY_SEED_DB_FILE="tests/screentest/seed.txt"
    export GO_DISCOVERY_VULN_DB="file:///pkgsite/tests/screentest/testdata/vulndb-v1"
    dcompose run --rm seeddb
    dcompose up --detach chromedp
    dcompose up --detach --force-recreate frontend
    dcompose run --rm --entrypoint bash go -c "
      go install golang.org/x/website/cmd/screentest@latest
      go run ./devtools/cmd/wait_available --timeout 120s frontend:8080 -- \
      $(echo $cmd)"
  elif [ "$env" = local ]; then
    if ! nc -z localhost 9222; then
      chromedp=$(runcmd docker run --detach --rm --network host --shm-size 8G \
        --name headless-shell chromedp/headless-shell:$chromedp_version)
      timeout 3s bash -c -- 'while ! nc -z localhost 9222; do sleep 1; done'
    fi
    if ! command -v screentest &> /dev/null; then
      runcmd go install golang.org/x/website/cmd/screentest@latest
    fi
    runcmd $cmd
  else
    dcompose up --detach chromedp
    dcompose run --rm --entrypoint bash go -c "
      go install golang.org/x/website/cmd/screentest@latest
      $(echo $cmd)"
  fi
}

main $@
