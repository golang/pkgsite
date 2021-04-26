-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE symbol_history (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
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

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON symbol_history
     FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON symbol_history IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE INDEX idx_symbol_history_package_path_id ON symbol_history(package_path_id);
CREATE INDEX idx_symbol_history_module_path_id ON symbol_history(module_path_id);
CREATE INDEX idx_symbol_history_symbol_name_id ON symbol_history(symbol_name_id);
CREATE INDEX idx_symbol_history_parent_symbol_name_id ON symbol_history(parent_symbol_name_id);
CREATE INDEX idx_symbol_history_package_symbol_id ON symbol_history(package_symbol_id);
CREATE INDEX idx_symbol_history_goos ON symbol_history(goos);
CREATE INDEX idx_symbol_history_goarch ON symbol_history(goarch);

COMMENT ON TABLE symbol_history IS
'TABLE symbol_history documents the first version when a symbol was introduced in a package.';

END;
