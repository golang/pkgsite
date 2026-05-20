#!/usr/bin/env -S bash -e

# Copyright 2026 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script dumps the schema of the local database
# into a file.
#
# You must have a local DB running at localhost:5432.
# That should be true if you've been testing with a DB.
#
# You must have pg_dump installed.
# If your installed version doesn't match your DB version,
# it may not work; set PG_DUMP to an alternate version.
# For example, pg_dump from v18 postgres will not work
# with a v14 database, but the v16 pg_dump will.
#
# GO_DISCOVERY_DATABASE_PASSWORD must be set to the
# password of the DB user "postgres".

outfile=db-schema.sql

pgdump=$PG_DUMP
if [[ -z $pgdump ]]; then
	pgdump=pg_dump
fi

db="dbname=discovery-db host=127.0.0.1 sslmode=disable port=5432 user=postgres password=$GO_DISCOVERY_DATABASE_PASSWORD"

echo '-- Copyright 2026 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.
' > $outfile

$pgdump -s "$db" >> $outfile
