#!/usr/bin/env -S bash -e

# Copyright 2019 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# Drop all test databases, when migrations are beyond repair.

run_pg() {
  PGPASSWORD="${GO_DISCOVERY_DATABASE_TEST_PASSWORD}" \
    psql -U postgres -h localhost -c "$1"
}

run_pg "DROP DATABASE IF EXISTS discovery_frontend_test;"
run_pg "DROP DATABASE IF EXISTS discovery_integration_test;"
run_pg "DROP DATABASE IF EXISTS discovery_postgres_test;"
run_pg "DROP DATABASE IF EXISTS discovery_worker_test;"
run_pg 'DROP DATABASE IF EXISTS "discovery_postgres_test-0";'
run_pg 'DROP DATABASE IF EXISTS "discovery_postgres_test-1";'
run_pg 'DROP DATABASE IF EXISTS "discovery_postgres_test-2";'
run_pg 'DROP DATABASE IF EXISTS "discovery_postgres_test-3";'
