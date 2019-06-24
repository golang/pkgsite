-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE licenses
  ADD COLUMN types TEXT[],
  ALTER COLUMN type DROP NOT NULL;

UPDATE licenses SET types = ARRAY[type];

END;
