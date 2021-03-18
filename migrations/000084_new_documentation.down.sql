-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation_symbols DROP CONSTRAINT IF EXISTS documentation_symbols_documentation_id_fkey;

ALTER TABLE documentation_symbols DROP CONSTRAINT IF EXISTS documentation_symbols_pkey;
ALTER TABLE documentation_symbols RENAME COLUMN id TO id_bigint;
ALTER TABLE documentation_symbols ALTER COLUMN id_bigint DROP IDENTITY IF EXISTS;
ALTER TABLE documentation_symbols ADD COLUMN id INTEGER PRIMARY KEY;

ALTER TABLE package_symbols DROP CONSTRAINT IF EXISTS package_symbols_pkey CASCADE;
ALTER TABLE package_symbols RENAME COLUMN id TO id_bigint;
ALTER TABLE package_symbols ALTER COLUMN id_bigint DROP IDENTITY IF EXISTS;
ALTER TABLE package_symbols ADD COLUMN id INTEGER PRIMARY KEY;

DROP TABLE new_documentation;

END;
