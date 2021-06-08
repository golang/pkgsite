-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE imports (
    unit_id BIGINT NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    to_path_id BIGINT NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    PRIMARY KEY (unit_id, to_path_id)
);

COMMENT ON TABLE imports IS
'TABLE imports contains the imports for a package in the units table.
The package represented by unit_id imports to_path_id.
We do not store the version and module at which to_path is imported because it is hard to compute.';

CREATE INDEX idx_imports_to_path_id ON imports USING btree (to_path_id);

END;
