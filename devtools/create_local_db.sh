#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for creating a new database locally.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

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

echo "CREATE DATABASE \"$database_name\" \
        OWNER = $database_user \
        TEMPLATE=template0 \
        LC_COLLATE = 'C' \
        LC_CTYPE = 'C';" | psql postgresql://$database_user:$database_password@$database_host:5432?sslmode=disable

