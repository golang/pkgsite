-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE symbol_history;
CREATE TABLE symbol_history (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    package_path_id INTEGER NOT NULL,
    module_path_id INTEGER NOT NULL,
    symbol_id INTEGER NOT NULL,
    parent_symbol_id INTEGER NOT NULL,
    since_version TEXT NOT NULL,
    section symbol_section NOT NULL,
    synopsis text NOT NULL,
    type symbol_type,
    goos goos NOT NULL,
    goarch goarch NOT NULL,

    UNIQUE(package_path_id, module_path_id, symbol_id, goos, goarch),
    FOREIGN KEY (parent_symbol_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE
);
DROP TABLE symbols;

END;
