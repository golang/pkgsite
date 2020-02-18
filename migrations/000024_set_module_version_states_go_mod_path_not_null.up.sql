-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

UPDATE module_version_states SET go_mod_path = '' WHERE go_mod_path IS NULL;

ALTER TABLE module_version_states
	ALTER COLUMN go_mod_path SET NOT NULL,
	ALTER COLUMN go_mod_path SET DEFAULT '';

END;
