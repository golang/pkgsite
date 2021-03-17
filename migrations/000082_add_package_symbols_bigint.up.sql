-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation_symbols DROP CONSTRAINT documentation_symbols_package_symbol_id_fkey;
ALTER TABLE package_symbols ADD COLUMN id_bigint bigint;

END;
