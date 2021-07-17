#!/usr/bin/env bash

# Copyright 2021 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

source devtools/docker.sh || { echo "Are you at repo root?"; exit 1; }

set -e

# Script for running a Go docker image, when go1.15+ is not available locally.
#
# This is can used when Go is not installed on a machine (such as in CI by
# kokoro, which is on go version go1.12 linux/amd64).
#c
# It mounts the pwd into a volume in the container at /pkgsite,
# and sets the working directory in the container to /pkgsite.

gocmd="dockercompose run go"
if type go > /dev/null; then
  # pkgsite requires go1.15 or higher. If that's installed on the machine, just
  # use the local go since it will be faster.
  # kokoro run go1.12.
  #
  # This awk program splits the third whitespace-separated field
  # (e.g. "go1.15.5") on the dot character and prints the second part.
  v=`go version | awk '{split($3, parts, /\./); print parts[2]}'`
  if (( v >= 15 )); then
    gocmd=go
  fi;
fi;

$gocmd $@

