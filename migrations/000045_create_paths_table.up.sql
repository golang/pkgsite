-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE paths (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    path TEXT NOT NULL
);
COMMENT ON TABLE paths IS
'TABLE paths contains the path string for every path in the units table.';

ALTER TABLE units ADD COLUMN path_id INTEGER;

END;
