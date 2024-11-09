#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

screentest_version=v0.0.0-20241108174919-3a761022ad6f
 
# This should match the version we are using in devtools/docker/compose.yaml.
chromedp_version=97.0.4692.71

chromedp_port=9222
frontend_port=8080
postgres_port=5432

usage() {
  >&2 cat <<EOUSAGE
Usage: $0 [OPTIONS] [ci|local|exp|dev|staging|prod]

  [ci]
    Run tests against a local server with a seeded database. This is what runs in
    CI/kokoro and should always pass on master.

  [local]
    Run ci tests without using 'docker compose'.
    Docker is used to run Postgres and headless Chrome, but the frontend and screentest
    binaries are run outside of docker. The database is created and seeded if necessary,
    but not brought down, so subsequent runs can reuse its state. Headless Chrome is
    also left running, because it takes a while to bring it down. Use 'docker container list'
    followed by 'docker container stop' to stop these docker containers.
    This is a good choice for testing locally before mailing a CL.

  [exp|dev|staging|prod]
    Run the tests against live instance of the given env. Use to verify that there
    are no unexpected changes after a deploy is complete.

Options:

  -concurrency <N>
    Set the number of testcases to run concurrently. Defaults to 1. Setting this too
    high in lower memory environments may cause instability in tests.

  -idtoken <TOKEN>
    Provide an identity token to pass to servers that require one.
    Generate a token with 'gcloud auth print-identity-token'.

  -rm
    Remove containers and volumes after tests are finished.
    You can provide this with no command-line argument to remove resources from a previous
    command that did not specify -rm.

  -update
    Recapture every snapshot during this test run.


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

rm=false
env=

cleanup() {
  if [[ $env != local ]]; then
    dcompose stop
    if $rm; then
      dcompose down --volumes --remove-orphans
    fi
  fi
  if [ ! -z $frontend_pid ]; then
    # The pid we captured is that of the 'go run' command; we want to kill its child.
    runcmd pkill --parent $frontend_pid
  fi
}

main() {
  trap cleanup EXIT
  local concurrency
  local idtoken
  local update
  while [[ $1 = -* ]]; do
    case "$1" in
      -concurrency)
        shift
        concurrency="-c $1"
        ;;
     -idtoken)
        shift
        idtoken=$1
        ;;
     -update)
        update="-u"
        ;;
     -rm)
        rm=true
        ;;
      *)
        usage
        exit 1
    esac
    shift
  done

  # -rm by itself brings down previous containers (see cleanup).
  if [[ $1 = '' && $rm ]]; then
    exit 0
  fi

  env=$1
  local debugger_url="-d ws://localhost:$chromedp_port"
  local vars
  case $env in
    ci)
      debugger_url="-d ws://chromedp:$chromedp_port"
      vars="-v Origin:http://frontend:$frontend_port"
      ;;
    exp|dev|staging)
      debugger_url="-d ws://chromedp:$chromedp_port"
      vars="-v Origin:https://$env-pkg.go.dev,QuotaBypass:$GO_DISCOVERY_E2E_QUOTA_BYPASS,Token:$idtoken"
      ;;
    prod)
      vars="-v Origin:https://pkg.go.dev,QuotaBypass:$bypass"
      ;;
    local) ;;
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
      go install golang.org/x/website/cmd/screentest@$screentest_version
      go run ./devtools/cmd/wait_available --timeout 120s frontend:$frontend_port -- \
      $(echo $cmd)"
  elif [[ "$env" == local ]]; then
    run_locally $concurrency $update
  else
    dcompose up --detach chromedp
    dcompose run --rm --entrypoint bash go -c "
      go install golang.org/x/website/cmd/screentest@$screentest_version
      $(echo $cmd)"
  fi
}


# run_locally: run outside of the docker compose network, but use
# docker for each component.
run_locally() {
  local concurrency=$1
  local update=$2

  if ! command -v screentest &> /dev/null; then
    runcmd go install golang.org/x/website/cmd/screentest@screentest_version
  fi

  export GO_DISCOVERY_DATABASE_NAME=discovery-db
  export GO_DISCOVERY_DATABASE_HOST=localhost
  export GO_DISCOVERY_DATABASE_PORT=$postgres_port
  export GO_DISCOVERY_DATABASE_USER=postgres
  export GO_DISCOVERY_DATABASE_PASSWORD=postgres
  export GO_DISCOVERY_LOG_LEVEL=warning
  export GO_DISCOVERY_VULN_DB=file:///$PWD/tests/screentest/testdata/vulndb-v1

  if ! listening $postgres_port; then
    info setting up postgres DB
    runcmd docker run --detach \
      -e POSTGRES_DB=$GO_DISCOVERY_DATABASE_NAME \
      -e POSTGRES_USER=$GO_DISCOVERY_DATABASE_USER \
      -e POSTGRES_PASSWORD=$GO_DISCOVERY_DATABASE_PASSWORD \
      -e LANG=C \
      -p $postgres_port:$postgres_port \
      --rm \
      postgres:11.12
    wait_for $postgres_port
    # Postgres can take some time to start up even after it is listening to the port.
    runcmd sleep 4
    runcmd go run ./devtools/cmd/db create
    runcmd go run ./devtools/cmd/db migrate
  fi

  info seeding DB
  go run ./devtools/cmd/seeddb -seed tests/screentest/seed.txt

  if ! listening $frontend_port; then
    info starting frontend

    go run ./cmd/frontend -host localhost:$frontend_port &
    wait_for $frontend_port
    frontend_pid=$!
  fi
  if ! listening $chromedp_port; then
    info starting chromedp
    runcmd docker run --detach --rm --network host --shm-size 8G \
          --name headless-shell chromedp/headless-shell:$chromedp_version
    wait_for $chromedp_port
  fi

  info "running screentest"
  screentest $concurrency $update \
    -v Origin:http://localhost:$frontend_port \
    -d ws://localhost:$chromedp_port \
    'tests/screentest/testcases.*'
}

listening() {
	nc -z localhost $1
}

wait_for() {
	timeout 5s bash -c -- "while ! nc -z localhost $1; do sleep 1; done"
}

main $@
