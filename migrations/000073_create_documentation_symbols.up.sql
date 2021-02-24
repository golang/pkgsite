-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE documentation_symbols (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    documentation_id INTEGER NOT NULL,
    package_symbol_id INTEGER NOT NULL,

    UNIQUE(documentation_id, package_symbol_id),
    -- TODO: add FK to documentation.id in a future migration, once the UNIQUE
    -- constraint has been added.
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE
);
COMMENT ON TABLE documentation_symbols IS
'TABLE documentation_symbols contains symbols for a given row in the documentation table.';

END;
