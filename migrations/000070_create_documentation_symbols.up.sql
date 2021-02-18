-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE documentation_symbols (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    documentation_id INTEGER NOT NULL,
    package_symbol_id INTEGER NOT NULL,

    UNIQUE(documentation_id, package_symbol_id),
    FOREIGN KEY (documentation_id) REFERENCES documentation(id) ON DELETE CASCADE,
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE
);
COMMENT ON TABLE documentation_symbols IS
'TABLE documentation_symbols contains symbols for a given row in the documentation table.';
COMMENT ON COLUMN documentation_symbols.documentation_id IS
'COLUMN documentation_symbols.documentation_id is used to join with the documentation table to obtain (unit_id, goos, goarch).';

END;
