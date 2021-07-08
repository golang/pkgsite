#!/usr/bin/env -S bash -e

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for dropping and creating a new database locally using docker.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

docker-compose -f devtools/docker/docker-compose.yaml stop
docker-compose -f devtools/docker/docker-compose.yaml down --remove-orphans
docker-compose -f devtools/docker/docker-compose.yaml up -d db
docker-compose -f devtools/docker/docker-compose.yaml run migrate
