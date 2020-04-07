-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE readmes (
    path_id INTEGER NOT NULL PRIMARY KEY REFERENCES paths(id) ON DELETE CASCADE,
    filename text NOT NULL,
    contents text NOT NULL
);
COMMENT ON TABLE readmes IS
'TABLE readmes contains README files at a given path.';

CREATE TABLE documentation (
    path_id INTEGER NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    goos text NOT NULL,
    goarch text NOT NULL,
    synopsis text NOT NULL,
    html text NOT NULL,
    PRIMARY KEY (path_id, goos, goarch)
);
COMMENT ON TABLE documentation IS
'TABLE documentation contains documentation for packages in the database.';

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
