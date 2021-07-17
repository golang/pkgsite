#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Drop all test databases, when migrations are beyond repair.
for dbname in \
    discovery_frontend_test \
    discovery_frontend_test \
    discovery_integration_test \
    discovery_postgres_test \
    discovery_worker_test \
    "discovery_postgres_test-0" \
    "discovery_postgres_test-1" \
    "discovery_postgres_test-2" \
    "discovery_postgres_test-3"
do
    GO_DISCOVERY_DATABASE_NAME=$dbname GO_DISCOVERY_LOG_LEVEL=info go run devtools/cmd/db/main.go drop
done
