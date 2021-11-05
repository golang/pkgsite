#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 PROJECT_ID

  Clone the private repo from Cloud Source repos, then generate a build tag
  for the deployment and an ID token for IAP requests.

EOUSAGE
  exit 1
}

main() {
  local project_id=$1
  gcloud source repos clone private private
  source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }
  (cd private && docker_image_tag > ../_BUILD_TAG)
  private/devtools/idtoken.sh $project_id > _ID_TOKEN
}

main $@
