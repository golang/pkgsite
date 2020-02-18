-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE module_version_states
	ALTER COLUMN status DROP NOT NULL,
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN error DROP NOT NULL,
	ALTER COLUMN error DROP DEFAULT;

END;
