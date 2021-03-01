-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE latest_module_versions (
    module_path_id INTEGER NOT NULL PRIMARY KEY,
    raw_version TEXT NOT NULL,
    cooked_version TEXT NOT NULL,
    good_version TEXT NOT NULL,
    raw_go_mod_bytes BYTEA NOT NULL,

    FOREIGN KEY (module_path_id) REFERENCES paths(id) ON DELETE CASCADE
);

COMMENT ON TABLE latest_module_versions IS
'TABLE latest_module_versions holds the latest versions of a module.';

COMMENT ON COLUMN latest_module_versions.raw_version IS
'COLUMN raw_version is the latest version of the module, ignoring retractions.';

COMMENT ON COLUMN latest_module_versions.cooked_version IS
'COLUMN cooked_version is the latest unretracted version of the module.';

COMMENT ON COLUMN latest_module_versions.good_version IS
'COLUMN good_version is the latest version of the module with a 2xx status.';

COMMENT ON COLUMN latest_module_versions.raw_go_mod_bytes IS
'COLUMN raw_go_mod_bytes is the contents of the go.mod file for the given module and raw version.';

END;
