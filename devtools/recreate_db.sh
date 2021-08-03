#!/usr/bin/env -S bash -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for dropping and creating a new database locally using docker.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

dockercompose stop
dockercompose down --remove-orphans
dockercompose up -d db
go run devtools/cmd/db/main.go drop
./devtools/create_local_db.sh
