-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

CREATE TABLE old_documentation (
    unit_id INTEGER NOT NULL,
    goos TEXT NOT NULL,
    goarch TEXT NOT NULL,
    synopsis TEXT NOT NULL,
    source BYTEA,
    PRIMARY KEY (unit_id, goos, goarch),
    FOREIGN KEY (unit_id) REFERENCES units(id) ON DELETE CASCADE
);

END;
