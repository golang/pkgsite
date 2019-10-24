-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

-- These functions are set as PARALLEL SAFE, since they do not modify any
-- database state and are safe to be run in parallel.
-- https://www.postgresql.org/docs/11/parallel-safety.html
-- https://www.postgresql.org/docs/11/sql-createfunction.html (see PARALLEL section)
ALTER FUNCTION hll_hash PARALLEL SAFE;
ALTER FUNCTION hll_zeros PARALLEL SAFE;
ALTER FUNCTION to_tsvector_parent_directories PARALLEL SAFE;

END;
