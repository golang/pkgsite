#!/usr/bin/env bash
set -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source private/devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }
source deploy/lib.sh

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
  dyn_config_bucket=$(yaml_env_var GO_DISCOVERY_CONFIG_BUCKET private/config/${env}.yaml)
  dyn_config_object=$(yaml_env_var GO_DISCOVERY_CONFIG_DYNAMIC private/config/${env}.yaml)
  dyn_config_gcs=gs://$dyn_config_bucket/$dyn_config_object
  gsutil cp private/config/$env-config.yaml $dyn_config_gcs
}

main $@
