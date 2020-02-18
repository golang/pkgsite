-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE module_version_states
	ALTER COLUMN status SET DEFAULT 0,
	ALTER COLUMN status SET NOT NULL,
	ALTER COLUMN error SET NOT NULL,
	ALTER COLUMN error SET DEFAULT '';

END;
