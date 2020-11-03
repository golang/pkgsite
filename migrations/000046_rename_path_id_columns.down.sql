-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE readmes RENAME COLUMN unit_id TO path_id;
ALTER TABLE documentation RENAME COLUMN unit_id TO path_id;
ALTER TABLE package_imports RENAME COLUMN unit_id TO path_id;

END;
