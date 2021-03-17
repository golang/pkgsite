-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation_symbols
    ADD CONSTRAINT documentation_symbols_package_symbol_id_fkey
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id);

ALTER TABLE package_symbols DROP COLUMN id_bigint;

END;
