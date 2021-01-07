#!/usr/bin/env -S bash -e

# Copyright 2020 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/lib.sh || { echo "Are you at repo root?"; exit 1; }

# Run npm install if node_modules directory does not exist.
if [ ! -d "node_modules" ]
then
  runcmd docker-compose -f devtools/config/docker-compose.yaml run --rm npm install --quiet
fi

# Run an npm command and capture the exit code.
runcmd docker-compose -f devtools/config/docker-compose.yaml run --rm npm $@
code=$EXIT_CODE

# Perform docker cleanup.
runcmd docker-compose -f devtools/config/docker-compose.yaml down --remove-orphans

# Exit with the code from the npm command.
exit $code
