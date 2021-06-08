-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE documentation RENAME unit_id TO big_unit_id;

ALTER TABLE documentation ADD COLUMN unit_id INTEGER;


END;
