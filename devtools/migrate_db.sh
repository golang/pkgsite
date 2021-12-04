#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

usage() {
  cat <<EOUSAGE
Usage: $0 [up|down|force|version] {#}"
EOUSAGE
}

database_user="postgres"
if [[ $GO_DISCOVERY_DATABASE_USER != "" ]]; then
  database_user=$GO_DISCOVERY_DATABASE_USER
fi
database_password=""
if [[ $GO_DISCOVERY_DATABASE_PASSWORD != "" ]]; then
  database_password=$GO_DISCOVERY_DATABASE_PASSWORD
fi
database_host="localhost"
if [[ $GO_DISCOVERY_DATABASE_HOST != "" ]]; then
  database_host=$GO_DISCOVERY_DATABASE_HOST
fi
database_name='discovery-db'
if [[ $GO_DISCOVERY_DATABASE_NAME != "" ]]; then
  database_name=$GO_DISCOVERY_DATABASE_NAME
fi
ssl_mode='disable'
if [[ $GO_DISCOVERY_DATABASE_SSL != "" ]]; then
  ssl_mode=$GO_DISCOVERY_DATABASE_SSL
fi

# Redirect stderr to stdout because migrate outputs to stderr, and we want
# to be able to use ordinary output redirection.
case "$1" in
  up|down|force|version)
    migrate \
      -source file:migrations \
      -database "postgresql://$database_user:$database_password@$database_host:5432/$database_name?sslmode=$ssl_mode" \
      "$@" 2>&1
    ;;
  *)
    usage
    exit 1
    ;;
esac
