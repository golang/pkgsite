-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE paths (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path            text NOT NULL,
    module_id       INTEGER NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    v1_path         text NOT NULL, -- used to compute package history; empty for non-packages
    name            text DEFAULT '' NOT NULL, -- empty for non-packages
    license_types   text[],
    license_paths   text[],
    redistributable boolean DEFAULT false NOT NULL,
    created_at      timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at      timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,

    UNIQUE (path, module_id)
);
COMMENT ON TABLE paths IS
'TABLE paths contains every module, package and directory path at every version.';

CREATE TRIGGER set_updated_at BEFORE INSERT OR UPDATE ON paths
    FOR EACH ROW EXECUTE PROCEDURE trigger_modify_updated_at();
COMMENT ON TRIGGER set_updated_at ON paths IS
'TRIGGER set_updated_at updates the value of the updated_at column to the current timestamp whenever a row is inserted or updated to the table.';

CREATE INDEX idx_paths_path ON paths (path);
COMMENT ON INDEX idx_paths_path is
'INDEX idx_paths_path is used to get path information from a path.';

CREATE INDEX idx_paths_v1_path ON paths USING btree (v1_path);
COMMENT ON INDEX idx_paths_v1_path IS
'INDEX idx_paths_v1_path is used to get all of the packages in a series.';


END;
