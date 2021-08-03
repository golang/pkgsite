#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for running docker compose with the relevant compose.yaml and .env file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

dockercompose $@
