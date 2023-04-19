#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  >&2 cat <<EOUSAGE

  Usage: $0 [exp|dev|staging|prod|beta]

  Copy the dynamic config to the cloud storage bucket for the given environment.

EOUSAGE
  exit 1
}

main() {
  local env=$1
  check_env $env
  dyn_config_bucket=$(config_bucket $env)
  dyn_config_object=${env}-config.yaml
  dyn_config_gcs=gs://$dyn_config_bucket/$dyn_config_object
  runcmd gsutil cp private/config/$env-config.yaml $dyn_config_gcs
  dyn_exclude_gcs=gs://$dyn_config_bucket/config/excluded.txt
  runcmd gsutil cp private/config/excluded.txt $dyn_exclude_gcs
}

main $@
