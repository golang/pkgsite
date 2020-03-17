-- Copyright 2020 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

-- This migration does not need to be run in a transaction.
-- Each ALTER TABLE happens atomically, and is idempotent.

ALTER TABLE alternative_module_paths
	ALTER COLUMN created_at type TIMESTAMP WITH TIME ZONE USING created_at AT TIME ZONE 'UTC';

ALTER TABLE excluded_prefixes
	ALTER COLUMN created_at type TIMESTAMP WITH TIME ZONE USING created_at AT TIME ZONE 'UTC';

ALTER TABLE modules
	ALTER COLUMN created_at type TIMESTAMP WITH TIME ZONE USING created_at AT TIME ZONE 'UTC',
	ALTER COLUMN updated_at type TIMESTAMP WITH TIME ZONE USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE version_map
	ALTER COLUMN created_at type TIMESTAMP WITH TIME ZONE USING created_at AT TIME ZONE 'UTC',
	ALTER COLUMN updated_at type TIMESTAMP WITH TIME ZONE USING updated_at AT TIME ZONE 'UTC';
