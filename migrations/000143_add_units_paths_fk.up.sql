-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE units
    ADD CONSTRAINT units_path_id_fkey
    FOREIGN KEY (path_id) REFERENCES paths(id)
    NOT VALID;

END;
