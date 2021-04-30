-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

DROP TABLE symbol_history;
CREATE TABLE symbol_history (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    package_path_id INTEGER NOT NULL,
    module_path_id INTEGER NOT NULL,
    symbol_name_id INTEGER NOT NULL,
    parent_symbol_name_id INTEGER NOT NULL,
    package_symbol_id INTEGER NOT NULL,
    since_version TEXT NOT NULL CHECK(since_version != ''),
    sort_version text NOT NULL,
    goos goos NOT NULL,
    goarch goarch NOT NULL,

    UNIQUE( package_path_id, module_path_id, symbol_name_id, goos, goarch),
    FOREIGN KEY (package_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_symbol_name_id) REFERENCES symbol_names(id) ON DELETE CASCADE,
    FOREIGN KEY (package_symbol_id) REFERENCES package_symbols(id) ON DELETE CASCADE
);

END;
