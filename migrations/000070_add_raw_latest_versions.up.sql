-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE raw_latest_versions (
    module_path_id INTEGER NOT NULL PRIMARY KEY,
    version TEXT NOT NULL,
    go_mod_bytes BYTEA NOT NULL,

    FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE
);

COMMENT ON TABLE raw_latest_versions IS
'TABLE raw_latest_versions holds the latest version of a module independent of retractions or processing status.';

COMMENT ON COLUMN raw_latest_versions.go_mod_bytes IS
'COLUMN go_mod_bytes is the contents of the go.mod file for the given module and version.';

END;
