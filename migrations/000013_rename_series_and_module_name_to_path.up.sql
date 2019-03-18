-- Copyright 2019 The Go Authors. All rights reserved.
-- Use of this source code is governed by a BSD-style
-- license that can be found in the LICENSE file.

BEGIN;

ALTER TABLE series RENAME COLUMN name TO path;

ALTER TABLE modules RENAME COLUMN name TO path;
ALTER TABLE modules RENAME COLUMN series_name TO series_path;
ALTER TABLE modules RENAME CONSTRAINT modules_series_name_fkey TO modules_series_path_fkey;

ALTER TABLE versions RENAME COLUMN name TO module_path;
ALTER TABLE versions RENAME CONSTRAINT versions_name_fkey TO versions_module_path_fkey;

ALTER TABLE dependencies RENAME COLUMN name TO module_path;
ALTER TABLE dependencies RENAME COLUMN dependency_name TO dependency_path;

ALTER TABLE version_logs RENAME COLUMN name TO module_path;

ALTER TABLE packages RENAME COLUMN name TO module_path;
ALTER TABLE packages ADD COLUMN name TEXT NOT NULL;
ALTER TABLE packages ALTER COLUMN synopsis DROP NOT NULL;
ALTER TABLE packages DROP CONSTRAINT IF EXISTS packages_name_version_key;
ALTER TABLE packages RENAME CONSTRAINT packages_name_fkey TO packages_module_path_fkey;

END;
