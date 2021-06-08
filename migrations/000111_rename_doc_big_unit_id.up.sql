-- Copyright 2021 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.


BEGIN;

ALTER TABLE documentation ALTER COLUMN big_unit_id SET NOT NULL;

ALTER TABLE documentation DROP COLUMN unit_id;

ALTER TABLE documentation RENAME big_unit_id TO unit_id;

END;
