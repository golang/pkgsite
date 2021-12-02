#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [exp|dev|staging|prod|beta] IDTOKEN

  Run the e2e tests against a live instance of the given environment. These tests
  will only pass against staging and prod.

EOUSAGE
  exit 1
}

main() {
  local env=$1
  local auth_token=$2
  check_env $env
  export CI=true
  export PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
  export GO_DISCOVERY_E2E_BASE_URL=$(frontend_url $env)
  export GO_DISCOVERY_E2E_AUTHORIZATION=$auth_token
  export GO_DISCOVERY_E2E_QUOTA_BYPASS=$QUOTA_BYPASS
  runcmd devtools/docker/compose.sh up -d chrome
  runcmd devtools/nodejs.sh npm ci
  runcmd devtools/nodejs.sh npx jest tests/e2e/*.test.ts --runInBand
}

main $@
