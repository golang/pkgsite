-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE package_symbols (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    package_path_id INTEGER NOT NULL,
    module_path_id INTEGER NOT NULL,
    symbol_name_id INTEGER NOT NULL,
    parent_symbol_name_id INTEGER NOT NULL,
    section symbol_section NOT NULL,
    type symbol_type NOT NULL,
    synopsis TEXT NOT NULL,

    FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE
);
COMMENT ON TABLE package_symbols IS
'TABLE package_symbols contains information that fully describes symbols that appear in a given package.';

COMMENT ON COLUMN package_symbols.parent_symbol_name_id IS
'COLUMN package_symbols.parent_symbol_name_id indicates the parent type for a symbol. If the symbol is the parent type, the parent_symbol_id will be equal to the symbol_id.';

DROP TABLE symbol_history;
CREATE TABLE symbol_history (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    package_symbol_id INTEGER NOT NULL,
    goos goos NOT NULL CHECK(goos != 'all'),
    goarch goarch NOT NULL CHECK(goarch != 'all'),
    since_version TEXT NOT NULL CHECK(since_version != ''),

    UNIQUE(package_symbol_id, goos, goarch),
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE
);
COMMENT ON TABLE symbol_history IS
'TABLE symbol_history documents the first version when a symbol was introduced in a package.';

END;
