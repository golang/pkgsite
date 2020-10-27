-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE imports (
    from_path text NOT NULL,
    from_module_path text NOT NULL,
    from_version text NOT NULL,
    to_path text NOT NULL,
    PRIMARY KEY (to_path, from_path, from_version, from_module_path),
    FOREIGN KEY (from_path, from_module_path, from_version)
        REFERENCES packages(path, module_path, version) ON DELETE CASCADE
);
COMMENT ON TABLE imports IS
'TABLE imports contains the imports for a package in the packages table. Package (from_path), in module (from_module_path) at version (from_version), imports package (to_path). We do not store the version and module at which to_path is imported because it is hard to compute.';

CREATE INDEX idx_imports_from_path_from_version ON imports (from_path, from_version);
COMMENT ON INDEX idx_imports_from_path_from_version IS
'INDEX idx_imports_from_path_from_version is used to improve performance of the imports tab.';

END;
