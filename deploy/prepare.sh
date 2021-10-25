#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 PROJECT_ID

  Clone the private repo from Cloud Source repos and generate a build tag
  for the deployment.

EOUSAGE
  exit 1
}

main() {
  gcloud source repos clone private private
  source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }
  (cd private && docker_image_tag > ../_BUILD_TAG)
}

main $@
