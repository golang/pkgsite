-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE documentation_symbols;
ALTER TABLE legacy_documentation_symbols RENAME TO documentation_symbols;
ALTER INDEX idx_legacy_documentation_symbols_documentation_id RENAME TO
    idx_documentation_symbols_documentation_id;
ALTER INDEX idx_legacy_documentation_symbols_package_symbol_id RENAME TO
    idx_documentation_symbols_package_symbol_id;

END;
