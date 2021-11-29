#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }
source deploy/lib.sh

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [exp|dev|staging|prod|beta] NAME:TAG

  Deploy a frontend image to Cloud Run for the given environment.

EOUSAGE
  exit 1
}

main() {
  local env=$1
  local image=$2
  check_env $env
  check_image $image
  gcloud run deploy --quiet --region us-central1 $env-frontend --image $image
  # If there was a rollback, `gcloud run deploy` will create a revision but
  # not point traffic to it. The following command ensures that the new revision
  # will get traffic.
  gcloud run services update-traffic $env-frontend --to-latest
  local tok=$(private/devtools/idtoken.sh $env)
  local hdr="Authorization: Bearer $tok"
  # Clear the redis cache
  curl -H "$hdr" $(worker_url $env)/clear-cache
}

main $@
