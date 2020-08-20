-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE modules ADD COLUMN status INTEGER;

COMMENT ON COLUMN modules.status IS
'COLUMN status describes the status of the module in the database. This status will match module_version_states.status.';

END;
