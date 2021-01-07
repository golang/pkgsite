#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Script for creating a new migration file.

migrate create -ext sql -dir migrations -seq $1
HEADER="-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- Write your migration here.

END;"
for f in $(ls migrations | tail -n 2); do echo "$HEADER" >> "migrations/$f"; done
