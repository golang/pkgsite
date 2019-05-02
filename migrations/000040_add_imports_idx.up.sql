-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- Creating an index concurrently cannot be done inside a transaction.
-- 
-- When this option is used, PostgreSQL will build the index without taking any
-- locks that prevent concurrent inserts, updates, or deletes on the table;
-- whereas a standard index build locks out writes (but not reads) on the table
-- until it's done. There are several caveats to be aware of when using this
-- option — see Building Indexes Concurrently.
-- See https://www.postgresql.org/docs/9.1/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY for more information.
CREATE INDEX CONCURRENTLY idx_imports_to_path ON imports(to_path, from_path);
