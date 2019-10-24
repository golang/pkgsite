-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER FUNCTION hll_hash PARALLEL UNSAFE;
ALTER FUNCTION hll_zeros PARALLEL UNSAFE;
ALTER FUNCTION to_tsvector_parent_directories PARALLEL UNSAFE;

END;
