# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Library of useful bash functions.

check_env() {
  local env=$1
  case "$env" in
    exp|dev|staging|prod|beta)
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

check_image() {
  local image=$1
  if ! gcloud container images describe $image; then
    echo
    echo "  Container $image not found."
    usage
  fi
}
