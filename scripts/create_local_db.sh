#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for creating a new database locally.

source scripts/lib.sh || { echo "Are you at repo root?"; exit 1; }

echo "CREATE DATABASE \"discovery-database\" \
        OWNER = postgres \
        TEMPLATE=template0 \
        LC_COLLATE = 'C' \
        LC_CTYPE = 'C';" | psql 'host=127.0.0.1 sslmode=disable user=postgres'
