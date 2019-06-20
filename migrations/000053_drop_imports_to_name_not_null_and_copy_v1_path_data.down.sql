-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE imports ADD COLUMN to_name TEXT;

ALTER TABLE packages ALTER COLUMN v1_path DROP NOT NULL;
UPDATE packages SET v1_path = NULL;

END;
