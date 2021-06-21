#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e

# Script for running a nodejs docker image.
# It passes env variables for e2e tests,
# mounts the pwd into a volume in the container at /pkgsite,
# and sets the working directory in the container to /pkgsite.

docker run --net=host --rm -t \
  -e CI \
  -e GO_DISCOVERY_E2E_BASE_URL \
  -e GO_DISCOVERY_E2E_AUTHORIZATION \
  -e GO_DISCOVERY_E2E_QUOTA_BYPASS \
  -e PUPPETEER_SKIP_CHROMIUM_DOWNLOAD \
  -v `pwd`:/pkgsite \
  -w /pkgsite  \
  node:14.17.0 $@
