#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [exp|dev|staging|prod|beta] NAME:TAG

  Deploy a worker image to gke for the given environment.

EOUSAGE
  exit 1
}

main() {
  local env=$1
  local image=$2
  check_env $env
  check_image $image
  runcmd docker run -v $(pwd)/private:/private cuelang/cue:0.4.0 cmd \
    -t env=$env \
    -t app=worker \
    -t workerImage=$image \
    print ./private/config/gke | gke-deploy apply -f - -c $env-pkgsite -l us-central1-a
}

main $@
