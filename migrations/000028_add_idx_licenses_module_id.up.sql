-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.
--
-- BEGIN and END were removed because of this error:
-- (details: pq: CREATE INDEX CONCURRENTLY cannot run inside a transaction block)
--	  * pq: current transaction is aborted, commands ignored until end of
--    transaction block in line 0: SELECT pg_advisory_unlock($1)

CREATE INDEX CONCURRENTLY idx_licenses_module_id ON licenses (module_id);
