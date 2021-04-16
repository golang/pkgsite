#!/usr/bin/env bash

# Copyright 2020 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e

# Script for running a nodejs docker image.

docker run -it -v `pwd`:/pkgsite -w /pkgsite  node:14.15.1 $@
