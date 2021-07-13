-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE package_imports (
    path_id INTEGER NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    to_path text NOT NULL,
    PRIMARY KEY (path_id, to_path)
);
CREATE INDEX idx_package_imports_to_path ON package_imports USING btree (to_path);
COMMENT ON TABLE package_imports IS
'TABLE package_imports contains the imports for a package in the paths table. The package represented by path_id imports to_path. We do not store the version and module at which to_path is imported because it is hard to compute.

This table will be renamed to imports, once the current imports table has been deprecated.';

END;
